package controller

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

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

const oidcClientFinalizer = "pocketid.tobiash.github.io/finalizer"

// PocketIDOIDCClientReconciler reconciles a PocketIDOIDCClient object
type PocketIDOIDCClientReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidoidcclients,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidoidcclients/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidoidcclients/finalizers,verbs=update
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusergroups,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *PocketIDOIDCClientReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{}
	if err := r.Get(ctx, req.NamespacedName, oidcClient); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	instance, err := ResolveInstanceReference(ctx, r.Client, oidcClient.Spec.InstanceRef, oidcClient.Namespace)
	if err != nil {
		return r.updateErrorStatus(ctx, oidcClient, "InstanceNotFound", err)
	}

	allowed, reason, err := ValidateCrossNamespaceReference(ctx, r.Client, instance, oidcClient.Namespace)
	if err != nil {
		return r.updateErrorStatus(ctx, oidcClient, "CrossNamespaceValidationFailed", err)
	}
	if !allowed {
		return r.updateErrorStatus(ctx, oidcClient, "CrossNamespaceDenied", fmt.Errorf("cross-namespace reference denied: %s", reason))
	}

	apiClient, err := r.getAPIClient(ctx, instance)
	if err != nil {
		return r.updateErrorStatus(ctx, oidcClient, "APIClientCreationFailed", err)
	}

	if !oidcClient.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, oidcClient, apiClient)
	}

	if !controllerutil.ContainsFinalizer(oidcClient, oidcClientFinalizer) {
		controllerutil.AddFinalizer(oidcClient, oidcClientFinalizer)
		if err := r.Update(ctx, oidcClient); err != nil {
			return ctrl.Result{}, err
		}
	}

	pocketIDClient, err := r.resolveOrCreateClient(ctx, apiClient, oidcClient)
	if err != nil {
		return r.updateErrorStatus(ctx, oidcClient, "ClientSyncFailed", err)
	}

	if err := r.ensureCredentialsSecret(ctx, apiClient, oidcClient, pocketIDClient, instance); err != nil {
		return ctrl.Result{}, err
	}

	clientID := pocketIDClient.ID
	credentialsSecretName := oidcClient.Status.CredentialsSecretName
	if err := r.Get(ctx, client.ObjectKeyFromObject(oidcClient), oidcClient); err != nil {
		return ctrl.Result{}, err
	}
	oidcClient.Status.ClientID = clientID
	oidcClient.Status.CredentialsSecretName = credentialsSecretName
	oidcClient.Status.Ready = true
	oidcClient.Status.Synced = true
	now := metav1.Now()
	oidcClient.Status.LastSyncTime = &now

	meta.SetStatusCondition(&oidcClient.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Synced",
		Message:            "OIDC Client synced successfully",
		ObservedGeneration: oidcClient.Generation,
	})

	if err := r.Status().Update(ctx, oidcClient); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *PocketIDOIDCClientReconciler) handleDeletion(ctx context.Context, oidcClient *pocketidv1alpha1.PocketIDOIDCClient, apiClient *pocketid.Client) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(oidcClient, oidcClientFinalizer) {
		if oidcClient.Status.ClientID != "" {
			if err := apiClient.DeleteOIDCClient(ctx, oidcClient.Status.ClientID); err != nil {
				return r.updateErrorStatus(ctx, oidcClient, "DeleteOIDCClientFailed", err)
			}
		}

		controllerutil.RemoveFinalizer(oidcClient, oidcClientFinalizer)
		if err := r.Update(ctx, oidcClient); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *PocketIDOIDCClientReconciler) resolveOrCreateClient(ctx context.Context, apiClient *pocketid.Client, oidcClient *pocketidv1alpha1.PocketIDOIDCClient) (*pocketid.OIDCClient, error) {
	var pocketIDClient *pocketid.OIDCClient

	if oidcClient.Status.ClientID != "" {
		var err error
		pocketIDClient, err = apiClient.GetOIDCClient(ctx, oidcClient.Status.ClientID)
		if err != nil {
			return nil, fmt.Errorf("GetOIDCClientFailed: %w", err)
		}
	} else {
		clients, err := apiClient.ListOIDCClients(ctx)
		if err != nil {
			return nil, fmt.Errorf("ListOIDCClientsFailed: %w", err)
		}
		for _, c := range clients {
			if c.Name == oidcClient.Spec.Name {
				pocketIDClient = &c
				break
			}
		}
	}

	targetClient := &pocketid.OIDCClient{
		Name:         oidcClient.Spec.Name,
		ID:           oidcClient.Spec.ClientID,
		IsPublic:     oidcClient.Spec.IsPublic,
		CallbackURLs: oidcClient.Spec.CallbackURLs,
	}

	if pocketIDClient == nil {
		logger := log.FromContext(ctx)
		logger.Info("Creating OIDC Client", "name", targetClient.Name)
		created, err := apiClient.CreateOIDCClient(ctx, targetClient)
		if err != nil {
			return nil, fmt.Errorf("CreateOIDCClientFailed: %w", err)
		}
		pocketIDClient = created
		oidcClient.Status.ClientID = created.ID
	} else {
		targetClient.ID = pocketIDClient.ID
		if pocketIDClient.Name != targetClient.Name || !equalStrings(pocketIDClient.CallbackURLs, targetClient.CallbackURLs) {
			logger := log.FromContext(ctx)
			logger.Info("Updating OIDC Client", "id", pocketIDClient.ID)
			updated, err := apiClient.UpdateOIDCClient(ctx, pocketIDClient.ID, targetClient)
			if err != nil {
				return nil, fmt.Errorf("UpdateOIDCClientFailed: %w", err)
			}
			pocketIDClient = updated
		}
	}

	return pocketIDClient, nil
}

func (r *PocketIDOIDCClientReconciler) ensureCredentialsSecret(ctx context.Context, apiClient *pocketid.Client, oidcClient *pocketidv1alpha1.PocketIDOIDCClient, pocketIDClient *pocketid.OIDCClient, instance *pocketidv1alpha1.PocketIDInstance) error {
	if oidcClient.Spec.IsPublic || oidcClient.Spec.CredentialsSecretRef == nil {
		return nil
	}

	secretName := oidcClient.Spec.CredentialsSecretRef.Name
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: oidcClient.Namespace}, secret)

	secretNeedsCreate := false
	if apierrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: oidcClient.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{},
		}
		if err := controllerutil.SetControllerReference(oidcClient, secret, r.Scheme); err != nil {
			return err
		}
		secretNeedsCreate = true
	} else if err != nil {
		return err
	}

	if _, ok := secret.Data["OIDC_CLIENT_SECRET"]; ok && !secretNeedsCreate {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Generating new client secret", "client", pocketIDClient.ID)
	clientSecret, err := apiClient.GenerateClientSecret(ctx, pocketIDClient.ID)
	if err != nil {
		return err
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["OIDC_CLIENT_ID"] = []byte(pocketIDClient.ID)
	secret.Data["OIDC_CLIENT_SECRET"] = []byte(clientSecret)
	secret.Data["OIDC_ISSUER_URL"] = []byte(instance.Spec.AppURL)

	if secretNeedsCreate {
		if err := r.Create(ctx, secret); err != nil {
			return err
		}
	} else {
		if err := r.Update(ctx, secret); err != nil {
			return err
		}
	}
	oidcClient.Status.CredentialsSecretName = secretName
	return nil
}

func (r *PocketIDOIDCClientReconciler) updateErrorStatus(ctx context.Context, oidcClient *pocketidv1alpha1.PocketIDOIDCClient, reason string, reconcileErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(reconcileErr, "Reconciliation failed", "reason", reason)

	oidcClient.Status.Ready = false
	oidcClient.Status.Synced = false
	meta.SetStatusCondition(&oidcClient.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            reconcileErr.Error(),
		ObservedGeneration: oidcClient.Generation,
	})

	if updateErr := r.Status().Update(ctx, oidcClient); updateErr != nil {
		logger.Error(updateErr, "Failed to update error status")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, reconcileErr
}

func (r *PocketIDOIDCClientReconciler) getAPIClient(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) (*pocketid.Client, error) {
	// Simple client creation logic, can be enhanced with caching

	// Get Secret
	secret := &corev1.Secret{}
	// Use the status secret name if available, else fallback or error
	secretName := instance.Status.StaticAPIKeySecretName
	if secretName == "" {
		secretName = instance.Name + "-api-key" // Fallback guess
	}

	if err := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: instance.Namespace}, secret); err != nil {
		return nil, fmt.Errorf("failed to get API key secret: %w", err)
	}

	apiKey := string(secret.Data["STATIC_API_KEY"])

	// URL construction
	// Access via internal service
	baseUrl := fmt.Sprintf("http://%s-svc.%s.svc.cluster.local", instance.Name, instance.Namespace)

	// If running locally (not in cluster), default to localhost forward
	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		// Check if we can reach the in-cluster DNS provided for convenience, otherwise fallback
		// Use env var for testing/custom local setup
		baseUrl = os.Getenv("POCKETID_DEV_API_URL")
		if baseUrl == "" {
			baseUrl = "http://localhost:8080"
		}
	}

	return pocketid.NewClient(baseUrl, apiKey), nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSorted := make([]string, len(a))
	bSorted := make([]string, len(b))
	copy(aSorted, a)
	copy(bSorted, b)
	slices.Sort(aSorted)
	slices.Sort(bSorted)
	for i := range aSorted {
		if aSorted[i] != bSorted[i] {
			return false
		}
	}
	return true
}

func (r *PocketIDOIDCClientReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pocketidv1alpha1.PocketIDOIDCClient{}).
		Complete(r)
}
