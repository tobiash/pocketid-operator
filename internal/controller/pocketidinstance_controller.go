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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
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

	// Get API key from secret
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: instance.Status.StaticAPIKeySecretName, Namespace: instance.Namespace}, secret)
	if err != nil {
		return fmt.Errorf("failed to get secret for initialization: %w", err)
	}
	apiKey := string(secret.Data["STATIC_API_KEY"])

	// Determine API URL
	// For production/in-cluster: use service name
	// For local development: might need localhost if port-forwarded
	baseUrl := fmt.Sprintf("http://%s-svc.%s.svc.cluster.local", instance.Name, instance.Namespace)

	// Small hack for local dev visibility: if it fails, try localhost:1411 if we are likely running locally
	// In a real operator, you'd handle this via cluster networking or specific config
	client := &http.Client{Timeout: 5 * time.Second}

	// Check if we should use dev API URL (when not running in cluster)
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		if devURL := os.Getenv("POCKETID_DEV_API_URL"); devURL != "" {
			baseUrl = devURL
		}
	}

	// 1. Check if users exist
	req, err := http.NewRequestWithContext(ctx, "GET", baseUrl+"/api/users", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		// If unreachable, we might be running locally. Try localhost fallback as a courtesy for this session.
		baseUrl = "http://localhost:1411"
		req, _ = http.NewRequestWithContext(ctx, "GET", baseUrl+"/api/users", nil)
		req.Header.Set("X-API-Key", apiKey)
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("instance API unreachable at %s: %w", baseUrl, err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to check users, status: %d", resp.StatusCode)
	}

	var usersResp struct {
		Data []interface{} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&usersResp); err != nil {
		return fmt.Errorf("failed to decode users response: %w", err)
	}

	// 2. If no users, create admin
	if len(usersResp.Data) == 0 {
		logger.Info("Initializing instance with admin user", "user", instance.Spec.InitialAdmin.Username)
		adminData := map[string]interface{}{
			"username":    instance.Spec.InitialAdmin.Username,
			"email":       instance.Spec.InitialAdmin.Email,
			"firstName":   instance.Spec.InitialAdmin.FirstName,
			"displayName": instance.Spec.InitialAdmin.DisplayName,
			"isAdmin":     true,
		}
		body, _ := json.Marshal(adminData)
		req, err = http.NewRequestWithContext(ctx, "POST", baseUrl+"/api/users", bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to create admin: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("failed to create admin, status: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}
		logger.Info("Initial admin created successfully")
	} else {
		logger.Info("Instance already has users, skipping initialization")
	}

	// 3. Mark as initialized
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

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instance.Namespace,
		},
		Data: map[string]string{
			"PUBLIC_APP_URL":   instance.Spec.AppURL,
			"TRUST_PROXY":      fmt.Sprintf("%t", instance.Spec.TrustProxy),
			"SESSION_DURATION": fmt.Sprintf("%d", instance.Spec.SessionDuration),
			"DB_PROVIDER":      instance.Spec.Database.Provider,
		},
	}

	// Add SMTP config if present
	if instance.Spec.SMTP != nil {
		desired.Data["SMTP_HOST"] = instance.Spec.SMTP.Host
		desired.Data["SMTP_PORT"] = fmt.Sprintf("%d", instance.Spec.SMTP.Port)
		desired.Data["SMTP_FROM"] = instance.Spec.SMTP.From
		desired.Data["SMTP_TLS"] = fmt.Sprintf("%t", instance.Spec.SMTP.TLS)
	}

	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: configMapName}, existing)
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

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: instance.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app.kubernetes.io/name":     "pocketid",
				"app.kubernetes.io/instance": instance.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(1411),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: serviceName}, existing)
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
	configMapName := fmt.Sprintf("%s-config", instance.Name)
	encryptionSecretName := fmt.Sprintf("%s-encryption-key", instance.Name)
	apiKeySecretName := fmt.Sprintf("%s-api-key", instance.Name)

	// Use provided secret refs or defaults
	if instance.Spec.EncryptionKeySecretRef != nil {
		encryptionSecretName = instance.Spec.EncryptionKeySecretRef.Name
	}
	if instance.Spec.StaticAPIKeySecretRef != nil {
		apiKeySecretName = instance.Spec.StaticAPIKeySecretRef.Name
	}

	replicas := int32(1)
	if instance.Spec.Replicas != nil {
		replicas = *instance.Spec.Replicas
	}

	image := "ghcr.io/pocket-id/pocket-id:latest"
	if instance.Spec.Image != "" {
		image = instance.Spec.Image
	}

	labels := map[string]string{
		"app.kubernetes.io/name":       "pocketid",
		"app.kubernetes.io/instance":   instance.Name,
		"app.kubernetes.io/managed-by": "pocketid-operator",
	}

	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stsName,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: fmt.Sprintf("%s-svc", instance.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "pocketid",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 1411,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							EnvFrom: []corev1.EnvFromSource{
								{
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: configMapName,
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: encryptionSecretName,
										},
									},
								},
								{
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: apiKeySecretName,
										},
									},
								},
							},
							Resources: instance.Spec.Resources,
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(1411),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt(1411),
									},
								},
								InitialDelaySeconds: 45,
								PeriodSeconds:       20,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/app/backend/data",
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: mustParseQuantity(instance.Spec.Storage.PVC),
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: stsName}, existing)
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

func mustParseQuantity(pvc *pocketidv1alpha1.PVCConfig) resource.Quantity {
	size := "1Gi"
	if pvc != nil && pvc.Size != "" {
		size = pvc.Size
	}
	q, err := resource.ParseQuantity(size)
	if err != nil {
		q, _ = resource.ParseQuantity("1Gi")
	}
	return q
}
