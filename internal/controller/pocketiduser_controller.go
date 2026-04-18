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
	"time"

	corev1 "k8s.io/api/core/v1"
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
	logger := log.FromContext(ctx)

	user := &pocketidv1alpha1.PocketIDUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the PocketIDInstance (may be cross-namespace)
	instance, err := ResolveInstanceReference(ctx, r.Client, user.Spec.InstanceRef, user.Namespace)
	if err != nil {
		logger.Error(err, "Failed to get PocketIDInstance")
		return ctrl.Result{}, err
	}

	// Validate cross-namespace reference
	allowed, reason, err := ValidateCrossNamespaceReference(ctx, r.Client, instance, user.Namespace)
	if err != nil {
		logger.Error(err, "Failed to validate cross-namespace reference")
		return ctrl.Result{}, err
	}
	if !allowed {
		logger.Info("Cross-namespace reference denied", "reason", reason)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Helper to create API client
	apiClient, err := r.getAPIClient(ctx, instance)
	if err != nil {
		logger.Error(err, "Failed to create API client")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Handle deletion
	if !user.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(user, userFinalizer) {
			if user.Status.UserID != "" {
				if err := apiClient.DeleteUser(ctx, user.Status.UserID); err != nil {
					logger.Error(err, "Failed to delete user from API")
				}
			}

			controllerutil.RemoveFinalizer(user, userFinalizer)
			if err := r.Update(ctx, user); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(user, userFinalizer) {
		controllerutil.AddFinalizer(user, userFinalizer)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync logic
	var pocketIDUser *pocketid.User

	// Check if user exists
	if user.Status.UserID != "" {
		pocketIDUser, err = apiClient.GetUser(ctx, user.Status.UserID)
		if err != nil {
			logger.Error(err, "Failed to get user from API")
			return ctrl.Result{}, err
		}
	} else {
		// Try to find by username
		users, err := apiClient.ListUsers(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}
		for _, u := range users {
			if u.Username == user.Spec.Username {
				pocketIDUser = &u
				break
			}
		}
	}

	// Build target user
	targetUser := &pocketid.User{
		Username:    user.Spec.Username,
		FirstName:   user.Spec.FirstName,
		LastName:    user.Spec.LastName,
		DisplayName: user.Spec.DisplayName,
		IsAdmin:     user.Spec.IsAdmin,
		Disabled:    user.Spec.Disabled,
		Locale:      user.Spec.Locale,
	}

	if user.Spec.Email != nil {
		targetUser.Email = *user.Spec.Email
	}

	if pocketIDUser == nil {
		// Create
		logger.Info("Creating user", "username", targetUser.Username)
		created, err := apiClient.CreateUser(ctx, targetUser)
		if err != nil {
			return ctrl.Result{}, err
		}
		pocketIDUser = created
		user.Status.UserID = created.ID
	} else {
		// Update if needed
		targetUser.ID = pocketIDUser.ID
		if needsUserUpdate(pocketIDUser, targetUser) {
			logger.Info("Updating user", "id", pocketIDUser.ID)
			updated, err := apiClient.UpdateUser(ctx, pocketIDUser.ID, targetUser)
			if err != nil {
				return ctrl.Result{}, err
			}
			pocketIDUser = updated
		}
	}

	// Sync group memberships
	if err := r.syncGroupMemberships(ctx, apiClient, user, pocketIDUser); err != nil {
		logger.Error(err, "Failed to sync group memberships")
		return ctrl.Result{}, err
	}

	// Handle onboarding email
	if user.Spec.SendOnboardingEmail && !user.Status.OnboardingEmailSent {
		logger.Info("Sending onboarding email", "userId", pocketIDUser.ID)
		resp, err := apiClient.SendOnboardingEmail(ctx, pocketIDUser.ID)
		if err != nil {
			logger.Error(err, "Failed to send onboarding email")
			// Don't fail reconciliation, just log
		} else {
			now := metav1.Now()
			user.Status.OnboardingEmailSent = true
			user.Status.OnboardingEmailSentAt = &now

			// Store one-time access link in secret if requested
			if user.Spec.OneTimeAccessSecretRef != nil && resp != nil {
				if err := r.storeOneTimeAccessLink(ctx, user, resp.OneTimeAccessLink); err != nil {
					logger.Error(err, "Failed to store one-time access link")
				}
			}
		}
	}

	// Update status
	now := metav1.Now()
	user.Status.Ready = true
	user.Status.Synced = true
	user.Status.LastSyncTime = &now
	user.Status.ObservedGeneration = user.Generation

	if err := r.Status().Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// needsUserUpdate checks if the user needs to be updated
func needsUserUpdate(current, target *pocketid.User) bool {
	if current.Username != target.Username {
		return true
	}
	if current.Email != target.Email {
		return true
	}
	if current.FirstName != target.FirstName {
		return true
	}
	if current.LastName != target.LastName {
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
	if current.Locale != target.Locale {
		return true
	}
	return false
}

// syncGroupMemberships synchronizes user group memberships
func (r *PocketIDUserReconciler) syncGroupMemberships(ctx context.Context, apiClient *pocketid.Client, user *pocketidv1alpha1.PocketIDUser, pocketIDUser *pocketid.User) error {
	logger := log.FromContext(ctx)

	// Get desired group IDs from UserGroupRefs
	desiredGroupIDs := make(map[string]bool)
	for _, groupRef := range user.Spec.UserGroupRefs {
		// Fetch the UserGroup CR
		group := &pocketidv1alpha1.PocketIDUserGroup{}
		if err := r.Get(ctx, types.NamespacedName{Name: groupRef.Name, Namespace: user.Namespace}, group); err != nil {
			logger.Error(err, "Failed to get UserGroup", "name", groupRef.Name)
			continue
		}
		if group.Status.GroupID != "" {
			desiredGroupIDs[group.Status.GroupID] = true
		}
	}

	// Get current group memberships
	currentGroupIDs := make(map[string]bool)
	if pocketIDUser.GroupIDs != nil {
		for _, gid := range pocketIDUser.GroupIDs {
			currentGroupIDs[gid] = true
		}
	}

	// Add user to groups they should be in
	for gid := range desiredGroupIDs {
		if !currentGroupIDs[gid] {
			logger.Info("Adding user to group", "userId", pocketIDUser.ID, "groupId", gid)
			if err := apiClient.AddUserToGroup(ctx, gid, pocketIDUser.ID); err != nil {
				logger.Error(err, "Failed to add user to group", "groupId", gid)
			}
		}
	}

	// Remove user from groups they shouldn't be in
	for gid := range currentGroupIDs {
		if !desiredGroupIDs[gid] {
			logger.Info("Removing user from group", "userId", pocketIDUser.ID, "groupId", gid)
			if err := apiClient.RemoveUserFromGroup(ctx, gid, pocketIDUser.ID); err != nil {
				logger.Error(err, "Failed to remove user from group", "groupId", gid)
			}
		}
	}

	return nil
}

// storeOneTimeAccessLink stores the one-time access link in a secret
func (r *PocketIDUserReconciler) storeOneTimeAccessLink(ctx context.Context, user *pocketidv1alpha1.PocketIDUser, link string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      user.Spec.OneTimeAccessSecretRef.Name,
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

	return r.Client.Patch(ctx, secret, client.Apply, client.FieldOwner("pocketid-operator"), client.ForceOwnership)
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
