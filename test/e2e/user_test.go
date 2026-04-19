package e2e

import (
	"context"
	"testing"
	"time"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestUserAndGroup(t *testing.T) {
	feature := features.New("User and Group Lifecycle").
		Assess("Create user group and wait for ready", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			group := &pocketidv1alpha1.PocketIDUserGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-group",
					Namespace: ns,
				},
				Spec: pocketidv1alpha1.PocketIDUserGroupSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: instanceName,
					},
					Name:         "developers",
					FriendlyName: "Development Team",
				},
			}
			if err := cfg.Client().Resources().Create(ctx, group); err != nil {
				t.Fatalf("failed to create group: %v", err)
			}
			t.Logf("Created user group")

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(group, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUserGroup)
				if !ok {
					return false
				}
				for _, cond := range obj.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatalf("group did not become ready: %v", err)
			}
			t.Logf("Group ready")

			if err := cfg.Client().Resources().Get(ctx, "test-group", ns, group); err != nil {
				t.Fatalf("failed to get group: %v", err)
			}
			if group.Status.GroupID == "" {
				t.Fatalf("GroupID not set")
			}
			t.Logf("GroupID: %s", group.Status.GroupID)

			return ctx
		}).
		Assess("Create user and wait for ready", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			email := "user1@test.com"
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user-1",
					Namespace: ns,
				},
				Spec: pocketidv1alpha1.PocketIDUserSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: instanceName,
					},
					Username:    "user1",
					Email:       &email,
					FirstName:   "Test",
					LastName:    "User",
					DisplayName: "Test User 1",
				},
			}
			if err := cfg.Client().Resources().Create(ctx, user); err != nil {
				t.Fatalf("failed to create user: %v", err)
			}
			t.Logf("Created user")

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(user, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUser)
				if !ok {
					return false
				}
				for _, cond := range obj.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatalf("user did not become ready: %v", err)
			}
			t.Logf("User ready")

			if err := cfg.Client().Resources().Get(ctx, "test-user-1", ns, user); err != nil {
				t.Fatalf("failed to get user: %v", err)
			}
			if user.Status.UserID == "" {
				t.Fatalf("UserID not set")
			}
			t.Logf("UserID: %s", user.Status.UserID)

			return ctx
		}).
		Assess("Create user with group membership", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			email := "user2@test.com"
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user-2",
					Namespace: ns,
				},
				Spec: pocketidv1alpha1.PocketIDUserSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: instanceName,
					},
					Username:    "user2",
					Email:       &email,
					FirstName:   "Test",
					LastName:    "User 2",
					DisplayName: "Test User 2",
					UserGroupRefs: []pocketidv1alpha1.LocalObjectReference{
						{Name: "test-group"},
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, user); err != nil {
				t.Fatalf("failed to create user: %v", err)
			}
			t.Logf("Created user with group membership")

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(user, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUser)
				if !ok {
					return false
				}
				for _, cond := range obj.Status.Conditions {
					if cond.Type == "Ready" && cond.Status == metav1.ConditionTrue {
						return true
					}
				}
				return false
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatalf("user did not become ready: %v", err)
			}
			t.Logf("User ready with group membership")

			return ctx
		}).
		Assess("Delete user with group membership", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{Name: "test-user-2", Namespace: ns},
			}
			if err := cfg.Client().Resources().Delete(ctx, user); err != nil {
				t.Fatalf("failed to delete user: %v", err)
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceDeleted(user), wait.WithTimeout(1*time.Minute))
			if err != nil {
				t.Fatalf("user not deleted: %v", err)
			}
			t.Logf("User deleted")

			return ctx
		}).
		Assess("Delete user without group", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{Name: "test-user-1", Namespace: ns},
			}
			if err := cfg.Client().Resources().Delete(ctx, user); err != nil {
				t.Fatalf("failed to delete user: %v", err)
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceDeleted(user), wait.WithTimeout(1*time.Minute))
			if err != nil {
				t.Fatalf("user not deleted: %v", err)
			}
			t.Logf("User deleted")

			return ctx
		}).
		Assess("Delete group", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			group := &pocketidv1alpha1.PocketIDUserGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-group", Namespace: ns},
			}
			if err := cfg.Client().Resources().Delete(ctx, group); err != nil {
				t.Fatalf("failed to delete group: %v", err)
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceDeleted(group), wait.WithTimeout(1*time.Minute))
			if err != nil {
				t.Fatalf("group not deleted: %v", err)
			}
			t.Logf("Group deleted")

			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
