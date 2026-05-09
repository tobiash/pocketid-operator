package controller

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var securityPolicyGVK = schema.GroupVersionKind{
	Group:   "gateway.envoyproxy.io",
	Version: "v1alpha1",
	Kind:    "SecurityPolicy",
}

func (r *PocketIDOIDCClientReconciler) ensureSecurityPolicy(ctx context.Context, oidcClient *pocketidv1alpha1.PocketIDOIDCClient, instance *pocketidv1alpha1.PocketIDInstance) error {
	logger := log.FromContext(ctx)

	if oidcClient.Spec.EnvoyGateway == nil || !oidcClient.Spec.EnvoyGateway.Enabled {
		if oidcClient.Status.SecurityPolicyName != "" {
			existing := &unstructured.Unstructured{}
			existing.SetGroupVersionKind(securityPolicyGVK)
			if err := r.Get(ctx, securityPolicyKey(oidcClient), existing); err == nil {
				if err := r.Delete(ctx, existing); err != nil {
					return fmt.Errorf("failed to delete SecurityPolicy: %w", err)
				}
				logger.Info("Deleted SecurityPolicy (envoyGateway disabled)")
			}
			oidcClient.Status.SecurityPolicyName = ""
		}
		return nil
	}

	if oidcClient.Status.ClientID == "" {
		return fmt.Errorf("cannot create SecurityPolicy: client ID not yet available")
	}
	if oidcClient.Status.CredentialsSecretName == "" {
		return fmt.Errorf("cannot create SecurityPolicy: credentials secret not yet available")
	}

	eg := oidcClient.Spec.EnvoyGateway
	if eg.HTTPRouteRef == nil {
		return fmt.Errorf("envoyGateway.httpRouteRef is required when envoyGateway is enabled")
	}

	routeNamespace := oidcClient.Namespace
	if eg.HTTPRouteRef.Namespace != "" {
		routeNamespace = eg.HTTPRouteRef.Namespace
	}

	route := &gatewayv1.HTTPRoute{}
	if err := r.Get(ctx, client.ObjectKey{Name: eg.HTTPRouteRef.Name, Namespace: routeNamespace}, route); err != nil {
		return fmt.Errorf("failed to get HTTPRoute %s/%s: %w", routeNamespace, eg.HTTPRouteRef.Name, err)
	}

	desired, err := buildSecurityPolicy(oidcClient, instance, route, oidcClient.Status.CredentialsSecretName, r.Scheme)
	if err != nil {
		return fmt.Errorf("failed to build SecurityPolicy: %w", err)
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(securityPolicyGVK)
	err = r.Get(ctx, securityPolicyKey(oidcClient), existing)
	if apierrors.IsNotFound(err) {
		logger.Info("Creating SecurityPolicy", "name", oidcClient.Name, "route", route.Name)
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create SecurityPolicy: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get SecurityPolicy: %w", err)
	} else {
		existing.Object["spec"] = desired.Object["spec"]
		if err := r.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update SecurityPolicy: %w", err)
		}
	}

	oidcClient.Status.SecurityPolicyName = oidcClient.Name
	return nil
}

func buildSecurityPolicy(
	oidcClient *pocketidv1alpha1.PocketIDOIDCClient,
	instance *pocketidv1alpha1.PocketIDInstance,
	route *gatewayv1.HTTPRoute,
	credentialsSecretName string,
	scheme *runtime.Scheme,
) (*unstructured.Unstructured, error) {
	if len(route.Spec.Hostnames) == 0 {
		return nil, fmt.Errorf("HTTPRoute %s/%s has no hostnames", route.Namespace, route.Name)
	}

	eg := oidcClient.Spec.EnvoyGateway
	callbackPath := "/oauth2/callback"
	if eg.CallbackPath != "" {
		callbackPath = eg.CallbackPath
	}
	logoutPath := "/logout"
	if eg.LogoutPath != "" {
		logoutPath = eg.LogoutPath
	}

	hostname := string(route.Spec.Hostnames[0])
	redirectURL := fmt.Sprintf("https://%s%s", hostname, callbackPath)

	appURL := strings.TrimRight(instance.Spec.AppURL, "/")

	sp := &unstructured.Unstructured{}
	sp.SetGroupVersionKind(securityPolicyGVK)
	sp.SetName(oidcClient.Name)
	sp.SetNamespace(oidcClient.Namespace)

	sp.Object["spec"] = map[string]any{
		"targetRefs": []any{
			map[string]any{
				"group": "gateway.networking.k8s.io",
				"kind":  "HTTPRoute",
				"name":  route.Name,
			},
		},
		"oidc": map[string]any{
			"provider": map[string]any{
				"issuer":                appURL,
				"authorizationEndpoint": appURL + "/authorize",
				"tokenEndpoint":         appURL + "/api/oidc/token",
			},
			"clientID": oidcClient.Status.ClientID,
			"clientSecret": map[string]any{
				"name": credentialsSecretName,
			},
			"redirectURL": redirectURL,
			"logoutPath":  logoutPath,
		},
	}

	if err := controllerutil.SetControllerReference(oidcClient, sp, scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return sp, nil
}

func securityPolicyKey(oidcClient *pocketidv1alpha1.PocketIDOIDCClient) client.ObjectKey {
	return client.ObjectKey{Name: oidcClient.Name, Namespace: oidcClient.Namespace}
}
