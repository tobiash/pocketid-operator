/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

const (
	AnnotationOIDCEnabled   = "pocketid.tobiash.github.io/oidc-enabled"
	AnnotationInstance      = "pocketid.tobiash.github.io/instance"
	AnnotationClientName    = "pocketid.tobiash.github.io/client-name"
	AnnotationRedirectPaths = "pocketid.tobiash.github.io/redirect-paths"
	AnnotationEnvoyGateway  = "pocketid.tobiash.github.io/envoy-gateway"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidoidcclients,verbs=get;list;watch;create;update;patch;delete

func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch HTTPRoute
	route := &gatewayv1.HTTPRoute{}
	if err := r.Get(ctx, req.NamespacedName, route); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check for enable annotation
	if route.Annotations[AnnotationOIDCEnabled] != "true" {
		return ctrl.Result{}, nil
	}

	// Determine Instance Name (may be cross-namespace: "namespace/name")
	instanceName := route.Annotations[AnnotationInstance]
	var instanceNamespace *string
	if instanceName == "" {
		// Try to find a single instance in the namespace
		var instances pocketidv1alpha1.PocketIDInstanceList
		if err := r.List(ctx, &instances, client.InNamespace(route.Namespace)); err != nil {
			return ctrl.Result{}, err
		}
		if len(instances.Items) == 1 {
			instanceName = instances.Items[0].Name
		} else if len(instances.Items) == 0 {
			logger.Info("No PocketIDInstances found in namespace, waiting")
			return ctrl.Result{}, nil
		} else {
			return ctrl.Result{}, fmt.Errorf("multiple PocketIDInstances found, please specify %s annotation", AnnotationInstance)
		}
	} else if strings.Contains(instanceName, "/") {
		// Parse namespace/name format
		parts := strings.SplitN(instanceName, "/", 2)
		ns := parts[0]
		instanceNamespace = &ns
		instanceName = parts[1]
	}

	// Resolve and validate instance reference
	instanceRef := pocketidv1alpha1.CrossNamespaceObjectReference{
		Name:      instanceName,
		Namespace: instanceNamespace,
	}
	instance, err := ResolveInstanceReference(ctx, r.Client, instanceRef, route.Namespace)
	if err != nil {
		logger.Error(err, "Failed to resolve instance reference")
		return ctrl.Result{}, err
	}

	// Validate cross-namespace access
	allowed, reason, err := ValidateCrossNamespaceReference(ctx, r.Client, instance, route.Namespace)
	if err != nil {
		logger.Error(err, "Failed to validate cross-namespace reference")
		return ctrl.Result{}, err
	}
	if !allowed {
		logger.Info("Cross-namespace reference denied", "reason", reason)
		return ctrl.Result{}, nil
	}

	// Determine Client Name
	clientName := route.Annotations[AnnotationClientName]
	if clientName == "" {
		clientName = fmt.Sprintf("%s-oidc", route.Name)
	}

	// Construct Redirect URIs
	var redirectURIs []string
	if paths := route.Annotations[AnnotationRedirectPaths]; paths != "" {
		// If paths valid, combine with hosts
		pathList := strings.Split(paths, ",")
		for _, host := range route.Spec.Hostnames {
			for _, path := range pathList {
				redirectURIs = append(redirectURIs, fmt.Sprintf("https://%s%s", host, strings.TrimSpace(path)))
			}
		}
	} else {
		// Default path: /oauth2/callback to match Envoy Gateway SecurityPolicy convention
		for _, host := range route.Spec.Hostnames {
			redirectURIs = append(redirectURIs, fmt.Sprintf("https://%s/oauth2/callback", host))
		}
	}

	if len(redirectURIs) == 0 {
		// No hostnames?
		logger.Info("No hostnames in HTTPRoute, cannot generate redirect URIs")
		return ctrl.Result{}, nil // Stop
	}

	// Define Desired Client
	desired := &pocketidv1alpha1.PocketIDOIDCClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientName,
			Namespace: route.Namespace,
		},
		Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
			Name:         clientName,
			InstanceRef:  instanceRef,
			CallbackURLs: redirectURIs,
			IsPublic:     false,
		},
	}

	if route.Annotations[AnnotationEnvoyGateway] == "true" {
		desired.Spec.EnvoyGateway = &pocketidv1alpha1.EnvoyGatewayConfig{
			Enabled: true,
			HTTPRouteRef: &pocketidv1alpha1.NamespacedObjectReference{
				Name:      route.Name,
				Namespace: route.Namespace,
			},
		}
	}

	if err := controllerutil.SetControllerReference(route, desired, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Create or Update
	existing := &pocketidv1alpha1.PocketIDOIDCClient{}
	err = r.Get(ctx, client.ObjectKey{Name: clientName, Namespace: route.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating PocketIDOIDCClient", "name", clientName)
		if err := r.Create(ctx, desired); err != nil {
			return ctrl.Result{}, err
		}
	} else if err != nil {
		return ctrl.Result{}, err
	} else {
		// Update if needed
		// For now, simple overwrite of spec fields we manage
		existing.Spec.CallbackURLs = desired.Spec.CallbackURLs
		existing.Spec.InstanceRef = desired.Spec.InstanceRef
		existing.Spec.EnvoyGateway = desired.Spec.EnvoyGateway
		if err := r.Update(ctx, existing); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1.HTTPRoute{}).
		Owns(&pocketidv1alpha1.PocketIDOIDCClient{}).
		Complete(r)
}
