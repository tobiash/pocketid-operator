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

const groupFinalizer = "pocketid.tobiash.github.io/group-finalizer"

// PocketIDUserGroupReconciler reconciles a PocketIDUserGroup object
type PocketIDUserGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusergroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusergroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidusergroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=pocketid.tobiash.github.io,resources=pocketidinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

func (r *PocketIDUserGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	group := &pocketidv1alpha1.PocketIDUserGroup{}
	if err := r.Get(ctx, req.NamespacedName, group); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch the PocketIDInstance (may be cross-namespace)
	instance, err := ResolveInstanceReference(ctx, r.Client, group.Spec.InstanceRef, group.Namespace)
	if err != nil {
		return r.updateErrorStatus(ctx, group, "InstanceNotFound", err)
	}

	// Validate cross-namespace reference
	allowed, reason, err := ValidateCrossNamespaceReference(ctx, r.Client, instance, group.Namespace)
	if err != nil {
		return r.updateErrorStatus(ctx, group, "CrossNamespaceValidationFailed", err)
	}
	if !allowed {
		return r.updateErrorStatus(ctx, group, "CrossNamespaceDenied", fmt.Errorf("cross-namespace reference denied: %s", reason))
	}

	// Helper to create API client
	apiClient, err := r.getAPIClient(ctx, instance)
	if err != nil {
		return r.updateErrorStatus(ctx, group, "APIClientCreationFailed", err)
	}

	// Handle deletion
	if !group.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(group, groupFinalizer) {
			if group.Status.GroupID != "" {
				if err := apiClient.DeleteGroup(ctx, group.Status.GroupID); err != nil {
					logger.Error(err, "Failed to delete group from API")
					return ctrl.Result{RequeueAfter: 10 * time.Second}, err
				}
			}

			controllerutil.RemoveFinalizer(group, groupFinalizer)
			if err := r.Update(ctx, group); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(group, groupFinalizer) {
		controllerutil.AddFinalizer(group, groupFinalizer)
		if err := r.Update(ctx, group); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Sync logic
	var pocketIDGroup *pocketid.UserGroup

	// Check if group exists
	if group.Status.GroupID != "" {
		pocketIDGroup, err = apiClient.GetGroup(ctx, group.Status.GroupID)
		if err != nil {
			return r.updateErrorStatus(ctx, group, "GetGroupFailed", err)
		}
	} else {
		// Try to find by name
		groups, err := apiClient.ListGroups(ctx)
		if err != nil {
			return r.updateErrorStatus(ctx, group, "ListGroupFailed", err)
		}
		for _, g := range groups {
			if g.Name == group.Spec.Name {
				pocketIDGroup = &g
				break
			}
		}
	}

	// Build target group
	targetGroup := &pocketid.UserGroupCreate{
		Name:         group.Spec.Name,
		FriendlyName: group.Spec.FriendlyName,
	}

	if pocketIDGroup == nil {
		logger.Info("Creating user group", "name", targetGroup.Name)
		created, err := apiClient.CreateGroup(ctx, targetGroup)
		if err != nil {
			return r.updateErrorStatus(ctx, group, "CreateGroupFailed", err)
		}
		pocketIDGroup = created
		group.Status.GroupID = created.ID
	} else {
		targetGroupUpdated := &pocketid.UserGroupCreate{
			Name:         group.Spec.Name,
			FriendlyName: group.Spec.FriendlyName,
		}
		if needsGroupUpdate(pocketIDGroup, targetGroupUpdated) {
			logger.Info("Updating user group", "id", pocketIDGroup.ID)
			updated, err := apiClient.UpdateGroup(ctx, pocketIDGroup.ID, targetGroupUpdated)
			if err != nil {
				return r.updateErrorStatus(ctx, group, "UpdateGroupFailed", err)
			}
			pocketIDGroup = updated
		}
	}

	// Update status
	groupID := pocketIDGroup.ID
	if err := r.Get(ctx, client.ObjectKeyFromObject(group), group); err != nil {
		return ctrl.Result{}, err
	}
	now := metav1.Now()
	group.Status.GroupID = groupID
	group.Status.Ready = true
	group.Status.Synced = true
	group.Status.LastSyncTime = &now
	group.Status.ObservedGeneration = group.Generation

	meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Synced",
		Message:            "User group synced successfully",
		ObservedGeneration: group.Generation,
	})

	if err := r.Status().Update(ctx, group); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *PocketIDUserGroupReconciler) updateErrorStatus(ctx context.Context, group *pocketidv1alpha1.PocketIDUserGroup, reason string, reconcileErr error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(reconcileErr, "Reconciliation failed", "reason", reason)

	group.Status.Ready = false
	group.Status.Synced = false
	meta.SetStatusCondition(&group.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            reconcileErr.Error(),
		ObservedGeneration: group.Generation,
	})

	if updateErr := r.Status().Update(ctx, group); updateErr != nil {
		logger.Error(updateErr, "Failed to update error status")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, reconcileErr
}

// needsGroupUpdate checks if the group needs to be updated
func needsGroupUpdate(current *pocketid.UserGroup, target *pocketid.UserGroupCreate) bool {
	if current.Name != target.Name {
		return true
	}
	if current.FriendlyName != target.FriendlyName {
		return true
	}
	return false
}

// getAPIClient creates a PocketID API client for the instance
func (r *PocketIDUserGroupReconciler) getAPIClient(ctx context.Context, instance *pocketidv1alpha1.PocketIDInstance) (*pocketid.Client, error) {
	return createAPIClient(ctx, r.Client, instance)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PocketIDUserGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pocketidv1alpha1.PocketIDUserGroup{}).
		Complete(r)
}
