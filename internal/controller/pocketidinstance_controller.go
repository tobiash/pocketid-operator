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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	"github.com/tobiash/pocketid-operator/internal/pocketid"
)

const (
	instanceFinalizer = "pocketid.tobiash.github.io/finalizer"
)

// PocketIDInstanceReconciler reconciles a PocketIDInstance object
type PocketIDInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the current state of the cluster closer to the desired state
func (r *PocketIDInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the PocketIDInstance
	instance := &pocketidv1alpha1.PocketIDInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("PocketIDInstance resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !instance.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, instance)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(instance, instanceFinalizer) {
		controllerutil.AddFinalizer(instance, instanceFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile resources
	if err := r.reconcileSecrets(ctx, instance); err != nil {
		return r.setDegradedCondition(ctx, instance, "SecretsReconcileFailed", err)
	}

	if err := r.reconcileConfigMap(ctx, instance); err != nil {
		return r.setDegradedCondition(ctx, instance, "ConfigMapReconcileFailed", err)
	}

	if err := r.reconcileService(ctx, instance); err != nil {
		return r.setDegradedCondition(ctx, instance, "ServiceReconcileFailed", err)
	}

	if err := r.reconcileStatefulSet(ctx, instance); err != nil {
		return r.setDegradedCondition(ctx, instance, "StatefulSetReconcileFailed", err)
	}

	// Initialize instance (Admin setup)
	if err := r.initializeInstance(ctx, instance); err != nil {
		// We don't want to block the entire reconciliation if initialization fails temporarily
		// but we should report it.
		instance.Status.Ready = false
		return r.setDegradedCondition(ctx, instance, "InitializationFailed", err)
	}

	// Update status
	return r.updateStatus(ctx, instance)
}

func (r *PocketIDInstanceReconciler) handleDeletion(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(instance, instanceFinalizer) {
		logger.Info("Cleaning up PocketIDInstance resources")
		// Resources will be garbage collected via OwnerReferences
		controllerutil.RemoveFinalizer(instance, instanceFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *PocketIDInstanceReconciler) reconcileSecrets(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) error {
	logger := log.FromContext(ctx)

	// Reconcile encryption key secret
	encryptionSecretName := fmt.Sprintf("%s-encryption-key", instance.Name)
	if instance.Spec.EncryptionKeySecretRef == nil {
		secret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: encryptionSecretName}, secret)
		if apierrors.IsNotFound(err) {
			logger.Info("Creating encryption key secret", "name", encryptionSecretName)
			key := generateRandomKey(32)
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      encryptionSecretName,
					Namespace: instance.Namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"ENCRYPTION_KEY": []byte(key),
				},
			}
			if err := controllerutil.SetControllerReference(instance, secret, r.Scheme); err != nil {
				return err
			}
			if err := r.Create(ctx, secret); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	// Reconcile static API key secret
	apiKeySecretName := fmt.Sprintf("%s-api-key", instance.Name)
	if instance.Spec.StaticAPIKeySecretRef == nil {
		secret := &corev1.Secret{}
		err := r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: apiKeySecretName}, secret)
		if apierrors.IsNotFound(err) {
			logger.Info("Creating static API key secret", "name", apiKeySecretName)
			key := generateRandomKey(32)
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      apiKeySecretName,
					Namespace: instance.Namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"STATIC_API_KEY": []byte(key),
				},
			}
			if err := controllerutil.SetControllerReference(instance, secret, r.Scheme); err != nil {
				return err
			}
			if err := r.Create(ctx, secret); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		// Update status with secret name
		instance.Status.StaticAPIKeySecretName = apiKeySecretName
	} else {
		instance.Status.StaticAPIKeySecretName = instance.Spec.StaticAPIKeySecretRef.Name
	}

	return nil
}

func (r *PocketIDInstanceReconciler) initializeInstance(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) error {
	logger := log.FromContext(ctx)

	// Skip if no initial admin config
	if instance.Spec.InitialAdmin == nil {
		return nil
	}

	// Skip if already initialized
	if meta.IsStatusConditionTrue(instance.Status.Conditions, "Initialized") {
		return nil
	}

	// Wait for pod to be ready before calling API
	if !instance.Status.Ready {
		return nil
	}

	apiClient, err := createAPIClientWithDevDefault(ctx, r.Client, instance, "http://localhost:1411")
	if err != nil {
		return fmt.Errorf("failed to create API client for initialization: %w", err)
	}

	users, err := apiClient.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list users for initialization: %w", err)
	}

	if len(users) == 0 {
		logger.Info("Initializing instance with admin user", "user", instance.Spec.InitialAdmin.Username)
		admin := &pocketid.User{
			Username:    instance.Spec.InitialAdmin.Username,
			Email:       &instance.Spec.InitialAdmin.Email,
			FirstName:   instance.Spec.InitialAdmin.FirstName,
			DisplayName: instance.Spec.InitialAdmin.DisplayName,
			IsAdmin:     true,
		}
		if err := apiClient.CreateUserWithoutResult(ctx, admin); err != nil {
			return fmt.Errorf("failed to create admin: %w", err)
		}
		logger.Info("Initial admin created successfully")
	} else {
		logger.Info("Instance already has users, skipping initialization")
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Initialized",
		Status:             metav1.ConditionTrue,
		Reason:             "AdminCreated",
		Message:            "Initial admin user has been created",
		ObservedGeneration: instance.Generation,
	})

	return nil
}

func (r *PocketIDInstanceReconciler) reconcileConfigMap(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) error {
	configMapName := fmt.Sprintf("%s-config", instance.Name)
	desired, err := desiredInstanceConfigMap(instance, ownerSetter(instance, r.Scheme))
	if err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: configMapName}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}

	// Update if needed
	existing.Data = desired.Data
	return r.Update(ctx, existing)
}

func (r *PocketIDInstanceReconciler) reconcileService(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) error {
	serviceName := fmt.Sprintf("%s-svc", instance.Name)
	desired, err := desiredInstanceService(instance, ownerSetter(instance, r.Scheme))
	if err != nil {
		return err
	}

	existing := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: serviceName}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}

	// Update service selector and ports
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	return r.Update(ctx, existing)
}

func (r *PocketIDInstanceReconciler) reconcileStatefulSet(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) error {
	stsName := instance.Name
	desired, err := desiredInstanceStatefulSet(instance, ownerSetter(instance, r.Scheme))
	if err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err = r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: stsName}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	} else if err != nil {
		return err
	}

	// Patch mutable fields to avoid conflict errors
	patch := client.MergeFrom(existing.DeepCopy())
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template = desired.Spec.Template
	return r.Patch(ctx, existing, patch)
}

func (r *PocketIDInstanceReconciler) updateStatus(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) (ctrl.Result, error) {
	// Re-fetch instance to avoid conflict error
	latest := &pocketidv1alpha1.PocketIDInstance{}
	if err := r.Get(ctx, client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}, latest); err != nil {
		return ctrl.Result{}, err
	}

	// Check StatefulSet status
	sts := &appsv1.StatefulSet{}
	stsName := latest.Name
	err := r.Get(ctx, client.ObjectKey{Namespace: latest.Namespace, Name: stsName}, sts)
	if err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Update conditions
	latest.Status.ObservedGeneration = latest.Generation
	latest.Status.InternalURL = fmt.Sprintf("http://%s-svc.%s.svc.cluster.local", latest.Name, latest.Namespace)
	latest.Status.StaticAPIKeySecretName = instance.Status.StaticAPIKeySecretName

	// Set Ready based on StatefulSet status
	if sts.Status.ReadyReplicas == *sts.Spec.Replicas && sts.Status.ReadyReplicas > 0 {
		latest.Status.Ready = true
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               pocketidv1alpha1.ConditionTypeAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             "AllReplicasReady",
			Message:            "All StatefulSet replicas are ready",
			ObservedGeneration: latest.Generation,
		})
	} else {
		latest.Status.Ready = false
		meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
			Type:               pocketidv1alpha1.ConditionTypeAvailable,
			Status:             metav1.ConditionFalse,
			Reason:             "ReplicasNotReady",
			Message:            fmt.Sprintf("Waiting for replicas: %d/%d ready", sts.Status.ReadyReplicas, *sts.Spec.Replicas),
			ObservedGeneration: latest.Generation,
		})
	}

	// Set Configured condition
	meta.SetStatusCondition(&latest.Status.Conditions, metav1.Condition{
		Type:               pocketidv1alpha1.ConditionTypeConfigured,
		Status:             metav1.ConditionTrue,
		Reason:             "ConfigurationValid",
		Message:            "Instance configuration is valid",
		ObservedGeneration: latest.Generation,
	})

	// Preserve Initialized condition if it exists in the original instance
	if initializedCond := meta.FindStatusCondition(instance.Status.Conditions, "Initialized"); initializedCond != nil {
		meta.SetStatusCondition(&latest.Status.Conditions, *initializedCond)
	}

	// Remove Degraded condition if it exists and reconciliation succeeded
	meta.RemoveStatusCondition(&latest.Status.Conditions, "Degraded")

	if err := r.Status().Update(ctx, latest); err != nil {
		return ctrl.Result{}, err
	}

	if !latest.Status.Ready {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

func (r *PocketIDInstanceReconciler) setDegradedCondition(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance, reason string, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(err, "Reconciliation failed", "reason", reason)

	instance.Status.Ready = false
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               pocketidv1alpha1.ConditionTypeDegraded,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            err.Error(),
		ObservedGeneration: instance.Generation,
	})

	if updateErr := r.Status().Update(ctx, instance); updateErr != nil {
		logger.Error(updateErr, "Failed to update status")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, updateErr
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *PocketIDInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pocketidv1alpha1.PocketIDInstance{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// Helper functions

func generateRandomKey(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}
