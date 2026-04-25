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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	"github.com/tobiash/pocketid-operator/internal/pocketid"
)

const userFinalizer = "pocketid.tobiash.github.io/user-finalizer"

// PocketIDUserReconciler reconciles a PocketIDUser object
type PocketIDUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusergroups,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

func (r *PocketIDUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	user := &pocketidv1alpha1.PocketIDUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	instance, err := ResolveInstanceReference(ctx, r.Client, user.Spec.InstanceRef, user.Namespace)
	if err != nil {
		return r.updateErrorStatus(ctx, user, "InstanceNotFound", err)
	}

	allowed, reason, err := ValidateCrossNamespaceReference(ctx, r.Client, instance, user.Namespace)
	if err != nil {
		return r.updateErrorStatus(ctx, user, "CrossNamespaceValidationFailed", err)
	}
	if !allowed {
		return r.updateErrorStatus(ctx, user, "CrossNamespaceDenied", fmt.Errorf("cross-namespace reference denied: %s", reason))
	}

	apiClient, err := r.getAPIClient(ctx, instance)
	if err != nil {
		return r.updateErrorStatus(ctx, user, "APIClientCreationFailed", err)
	}

	if !user.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, user, apiClient)
	}

	if !controllerutil.ContainsFinalizer(user, userFinalizer) {
		controllerutil.AddFinalizer(user, userFinalizer)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	pocketIDUser, err := r.resolveOrCreateUser(ctx, apiClient, user)
	if err != nil {
		return r.updateErrorStatus(ctx, user, "UserSyncFailed", err)
	}

	if err := r.syncGroupMemberships(ctx, apiClient, user, pocketIDUser); err != nil {
		return r.updateErrorStatus(ctx, user, "GroupSyncFailed", err)
	}

	r.handleOnboarding(ctx, apiClient, user, pocketIDUser, instance)

	userID := pocketIDUser.ID
	if err := r.Get(ctx, client.ObjectKeyFromObject(user), user); err != nil {
		return ctrl.Result{}, err
	}
	now := metav1.Now()
	user.Status.UserID = userID
	user.Status.Ready = true
	user.Status.Synced = true
	user.Status.LastSyncTime = &now
	user.Status.ObservedGeneration = user.Generation

	meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Synced",
		Message:            "User synced successfully",
		ObservedGeneration: user.Generation,
	})

	if err := r.Status().Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *PocketIDUserReconciler) handleDeletion(ctx context.Context, user *pocketidv1alpha1.PocketIDUser, apiClient *pocketid.Client) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(user, userFinalizer) {
		if user.Status.UserID != "" {
			if err := apiClient.DeleteUser(ctx, user.Status.UserID); err != nil {
				return r.updateErrorStatus(ctx, user, "DeleteUserFailed", err)
			}
		}

		controllerutil.RemoveFinalizer(user, userFinalizer)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *PocketIDUserReconciler) resolveOrCreateUser(ctx context.Context, apiClient *pocketid.Client, user *pocketidv1alpha1.PocketIDUser) (*pocketid.User, error) {
	var pocketIDUser *pocketid.User
	var err error

	if user.Status.UserID != "" {
		pocketIDUser, err = apiClient.GetUser(ctx, user.Status.UserID)
		if err != nil {
			return nil, fmt.Errorf("GetUserFailed: %w", err)
		}
		return pocketIDUser, nil
	}

	users, err := apiClient.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListUsersFailed: %w", err)
	}
	for _, u := range users {
		if u.Username == user.Spec.Username {
			pocketIDUser = &u
			break
		}
	}

	targetUser := &pocketid.User{
		Username:    user.Spec.Username,
		FirstName:   user.Spec.FirstName,
		DisplayName: user.Spec.DisplayName,
		IsAdmin:     user.Spec.IsAdmin,
		Disabled:    user.Spec.Disabled,
		Email:       user.Spec.Email,
	}
	if user.Spec.LastName != "" {
		targetUser.LastName = &user.Spec.LastName
	}
	if user.Spec.Locale != "" {
		targetUser.Locale = &user.Spec.Locale
	}

	if pocketIDUser == nil {
		created, err := apiClient.CreateUser(ctx, targetUser)
		if err != nil {
			return nil, fmt.Errorf("CreateUserFailed: %w", err)
		}
		pocketIDUser = created
		user.Status.UserID = created.ID
	} else {
		targetUser.ID = pocketIDUser.ID
		if needsUserUpdate(pocketIDUser, targetUser) {
			updated, err := apiClient.UpdateUser(ctx, pocketIDUser.ID, targetUser)
			if err != nil {
				return nil, fmt.Errorf("UpdateUserFailed: %w", err)
			}
			pocketIDUser = updated
		}
	}

	return pocketIDUser, nil
}

func (r *PocketIDUserReconciler) handleOnboarding(ctx context.Context, apiClient *pocketid.Client, user *pocketidv1alpha1.PocketIDUser, pocketIDUser *pocketid.User, instance *pocketidv1alpha1.PocketIDInstance) {
	logger := log.FromContext(ctx)

	secretName := r.resolveOnboardingSecretName(user)
	if secretName == "" || user.Status.OnboardingLinkCreated {
		return
	}

	logger.Info("Creating one-time access token", "userId", pocketIDUser.ID, "secret", secretName)
	resp, err := apiClient.CreateOneTimeAccessToken(ctx, pocketIDUser.ID)
	if err != nil {
		logger.Error(err, "Failed to create one-time access token")
		return
	}

	if resp != nil {
		link := fmt.Sprintf("%s/login/%s", instance.Spec.AppURL, resp.Token)
		if err := r.storeOneTimeAccessLink(ctx, user, secretName, link); err != nil {
			logger.Error(err, "Failed to store one-time access link")
			return
		}
	}

	user.Status.OnboardingLinkCreated = true

	if user.Spec.SendOnboardingEmail && !user.Status.OnboardingEmailSent {
		now := metav1.Now()
		user.Status.OnboardingEmailSent = true
		user.Status.OnboardingEmailSentAt = &now
	}
}

func (r *PocketIDUserReconciler) resolveOnboardingSecretName(user *pocketidv1alpha1.PocketIDUser) string {
	if user.Spec.OneTimeAccessSecretRef != nil {
		return user.Spec.OneTimeAccessSecretRef.Name
	}
	return user.Name + "-onboarding"
}

func (r *PocketIDUserReconciler) updateErrorStatus(ctx context.Context, user *pocketidv1alpha1.PocketIDUser, reason string, reconcileErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(reconcileErr, "Reconciliation failed", "reason", reason)

	user.Status.Ready = false
	user.Status.Synced = false
	meta.SetStatusCondition(&user.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            reconcileErr.Error(),
		ObservedGeneration: user.Generation,
	})

	if updateErr := r.Status().Update(ctx, user); updateErr != nil {
		logger.Error(updateErr, "Failed to update error status")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, reconcileErr
}

// needsUserUpdate checks if the user needs to be updated
func needsUserUpdate(current, target *pocketid.User) bool {
	if current.Username != target.Username {
		return true
	}
	if !stringPtrEqual(current.Email, target.Email) {
		return true
	}
	if current.FirstName != target.FirstName {
		return true
	}
	if !stringPtrEqual(current.LastName, target.LastName) {
		return true
	}
	if current.DisplayName != target.DisplayName {
		return true
	}
	if current.IsAdmin != target.IsAdmin {
		return true
	}
	if current.Disabled != target.Disabled {
		return true
	}
	if !stringPtrEqual(current.Locale, target.Locale) {
		return true
	}
	return false
}

func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// syncGroupMemberships synchronizes user group memberships
func (r *PocketIDUserReconciler) syncGroupMemberships(ctx context.Context, apiClient *pocketid.Client, user *pocketidv1alpha1.PocketIDUser, pocketIDUser *pocketid.User) error {
	logger := log.FromContext(ctx)

	desiredGroupIDs := make(map[string]bool)
	for _, groupRef := range user.Spec.UserGroupRefs {
		group := &pocketidv1alpha1.PocketIDUserGroup{}
		if err := r.Get(ctx, types.NamespacedName{Name: groupRef.Name, Namespace: user.Namespace}, group); err != nil {
			logger.Error(err, "Failed to get UserGroup", "name", groupRef.Name)
			continue
		}
		if group.Status.GroupID != "" {
			desiredGroupIDs[group.Status.GroupID] = true
		}
	}

	currentGroupIDs := make(map[string]bool)
	for _, ug := range pocketIDUser.UserGroups {
		currentGroupIDs[ug.ID] = true
	}

	needsUpdate := false
	for gid := range desiredGroupIDs {
		if !currentGroupIDs[gid] {
			needsUpdate = true
			break
		}
	}
	if !needsUpdate {
		for gid := range currentGroupIDs {
			if !desiredGroupIDs[gid] {
				needsUpdate = true
				break
			}
		}
	}

	if needsUpdate {
		groupIDs := make([]string, 0, len(desiredGroupIDs))
		for gid := range desiredGroupIDs {
			groupIDs = append(groupIDs, gid)
		}
		logger.Info("Updating user group memberships", "userId", pocketIDUser.ID, "groups", groupIDs)
		if err := apiClient.SetUserGroups(ctx, pocketIDUser.ID, groupIDs); err != nil {
			return err
		}
	}

	return nil
}

// storeOneTimeAccessLink stores the one-time access link in a secret
func (r *PocketIDUserReconciler) storeOneTimeAccessLink(ctx context.Context, user *pocketidv1alpha1.PocketIDUser, secretName, link string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: user.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"ONE_TIME_ACCESS_LINK": []byte(link),
		},
	}

	if err := controllerutil.SetControllerReference(user, secret, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: user.Namespace}, existing)
	if err != nil {
		return r.Create(ctx, secret)
	}
	existing.Data = secret.Data
	return r.Update(ctx, existing)
}

// getAPIClient creates a PocketID API client for the instance
func (r *PocketIDUserReconciler) getAPIClient(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) (*pocketid.Client, error) {
	return createAPIClient(ctx, r.Client, instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PocketIDUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pocketidv1alpha1.PocketIDUser{}).
		Complete(r)
}
