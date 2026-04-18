package e2e

import (
	"context"
	"testing"
	"time"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestUserAndGroupLifecycle(t *testing.T) {
	feature := features.New("User and Group Lifecycle").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create PocketID Instance
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance-users",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "http://pocketid-users.test",
					InitialAdmin: &pocketidv1alpha1.InitialAdminConfig{
						Email:       "admin@test.com",
						Username:    "admin",
						FirstName:   "Admin",
						DisplayName: "Admin User",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, instance); err != nil {
				t.Fatal(err)
			}

			// Wait for instance to be ready
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(instance, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDInstance)
				if !ok {
					return false
				}
				return obj.Status.Ready
			}), wait.WithTimeout(5*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Assess("Create User Group", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create a user group with custom claims
			group := &pocketidv1alpha1.PocketIDUserGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-group",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDUserGroupSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: "test-instance-users",
					},
					Name:         "developers",
					FriendlyName: "Development Team",
					CustomClaims: []pocketidv1alpha1.CustomClaim{
						{
							Key:   "team",
							Value: "dev",
						},
						{
							Key:   "role",
							Value: "developer",
						},
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, group); err != nil {
				t.Fatal(err)
			}

			// Wait for group to be ready
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(group, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUserGroup)
				if !ok {
					return false
				}
				return obj.Status.Ready && obj.Status.GroupID != ""
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			// Verify group has GroupID
			if err := cfg.Client().Resources().Get(ctx, group.Name, group.Namespace, group); err != nil {
				t.Fatal(err)
			}
			if group.Status.GroupID == "" {
				t.Error("Group GroupID is empty")
			}
			if !group.Status.Synced {
				t.Error("Group is not synced")
			}

			return ctx
		}).
		Assess("Create User without Group", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			email := "user1@test.com"
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user-1",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDUserSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: "test-instance-users",
					},
					Username:    "user1",
					Email:       &email,
					FirstName:   "Test",
					LastName:    "User",
					DisplayName: "Test User 1",
				},
			}
			if err := cfg.Client().Resources().Create(ctx, user); err != nil {
				t.Fatal(err)
			}

			// Wait for user to be ready
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(user, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUser)
				if !ok {
					return false
				}
				return obj.Status.Ready && obj.Status.UserID != ""
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			// Verify user has UserID
			if err := cfg.Client().Resources().Get(ctx, user.Name, user.Namespace, user); err != nil {
				t.Fatal(err)
			}
			if user.Status.UserID == "" {
				t.Error("User UserID is empty")
			}
			if !user.Status.Synced {
				t.Error("User is not synced")
			}

			return ctx
		}).
		Assess("Create User with Group Membership", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			email := "user2@test.com"
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user-2",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDUserSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: "test-instance-users",
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
				t.Fatal(err)
			}

			// Wait for user to be ready
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(user, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUser)
				if !ok {
					return false
				}
				return obj.Status.Ready && obj.Status.UserID != ""
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			// Verify user status
			if err := cfg.Client().Resources().Get(ctx, user.Name, user.Namespace, user); err != nil {
				t.Fatal(err)
			}
			if user.Status.UserID == "" {
				t.Error("User UserID is empty")
			}
			if !user.Status.Synced {
				t.Error("User is not synced")
			}

			return ctx
		}).
		Assess("Create User with Onboarding Secret", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			email := "user3@test.com"
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user-3",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDUserSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: "test-instance-users",
					},
					Username:            "user3",
					Email:               &email,
					FirstName:           "Test",
					LastName:            "User 3",
					DisplayName:         "Test User 3",
					SendOnboardingEmail: true,
					OneTimeAccessSecretRef: &pocketidv1alpha1.LocalObjectReference{
						Name: "user3-onboarding",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, user); err != nil {
				t.Fatal(err)
			}

			// Wait for user to be ready and onboarding email sent
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(user, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUser)
				if !ok {
					return false
				}
				return obj.Status.Ready && obj.Status.UserID != "" && obj.Status.OnboardingEmailSent
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			// Verify onboarding secret was created
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "user3-onboarding", Namespace: "default"},
			}
			if err := cfg.Client().Resources().Get(ctx, secret.Name, secret.Namespace, secret); err != nil {
				t.Fatal(err)
			}
			if len(secret.Data["ONE_TIME_ACCESS_LINK"]) == 0 {
				t.Error("Onboarding secret ONE_TIME_ACCESS_LINK is empty")
			}

			return ctx
		}).
		Assess("Update User Group Membership", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Add user1 to the group
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{Name: "test-user-1", Namespace: "default"},
			}
			if err := cfg.Client().Resources().Get(ctx, user.Name, user.Namespace, user); err != nil {
				t.Fatal(err)
			}

			// Update to add group membership
			user.Spec.UserGroupRefs = []pocketidv1alpha1.LocalObjectReference{
				{Name: "test-group"},
			}
			if err := cfg.Client().Resources().Update(ctx, user); err != nil {
				t.Fatal(err)
			}

			// Wait for sync (give it time to reconcile)
			time.Sleep(10 * time.Second)

			// Verify user is still ready
			if err := cfg.Client().Resources().Get(ctx, user.Name, user.Namespace, user); err != nil {
				t.Fatal(err)
			}
			if !user.Status.Ready {
				t.Error("User is not ready after group update")
			}

			return ctx
		}).
		Assess("Delete User", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Delete user3
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{Name: "test-user-3", Namespace: "default"},
			}
			if err := cfg.Client().Resources().Delete(ctx, user); err != nil {
				t.Fatal(err)
			}

			// Wait for deletion
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceDeleted(user), wait.WithTimeout(1*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Assess("Delete Group", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Delete the group
			group := &pocketidv1alpha1.PocketIDUserGroup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-group", Namespace: "default"},
			}
			if err := cfg.Client().Resources().Delete(ctx, group); err != nil {
				t.Fatal(err)
			}

			// Wait for deletion
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceDeleted(group), wait.WithTimeout(1*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Cleanup instance
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "test-instance-users", Namespace: "default"},
			}
			cfg.Client().Resources().Delete(ctx, instance)

			// Cleanup any remaining users
			user1 := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{Name: "test-user-1", Namespace: "default"},
			}
			cfg.Client().Resources().Delete(ctx, user1)

			user2 := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{Name: "test-user-2", Namespace: "default"},
			}
			cfg.Client().Resources().Delete(ctx, user2)

			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}

func TestCrossNamespaceUserAccess(t *testing.T) {
	feature := features.New("Cross-Namespace User Access").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create namespace for instance
			authNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "auth-system",
					Labels: map[string]string{
						"env": "test",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, authNs); err != nil {
				t.Fatal(err)
			}

			// Create PocketID Instance in auth-system namespace with cross-namespace allowed
			fromAll := pocketidv1alpha1.NamespacesFromAll
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shared-instance",
					Namespace: "auth-system",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "http://shared-auth.test",
					AllowedReferences: &pocketidv1alpha1.AllowedReferences{
						Namespaces: &pocketidv1alpha1.NamespacesFrom{
							From: &fromAll,
						},
					},
					InitialAdmin: &pocketidv1alpha1.InitialAdminConfig{
						Email:       "admin@test.com",
						Username:    "admin",
						FirstName:   "Admin",
						DisplayName: "Admin User",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, instance); err != nil {
				t.Fatal(err)
			}

			// Wait for instance to be ready
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(instance, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDInstance)
				if !ok {
					return false
				}
				return obj.Status.Ready
			}), wait.WithTimeout(5*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Assess("Create User in Different Namespace", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			authNs := "auth-system"
			email := "crossns@test.com"
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cross-ns-user",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDUserSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name:      "shared-instance",
						Namespace: &authNs,
					},
					Username:    "crossnsuser",
					Email:       &email,
					FirstName:   "Cross",
					LastName:    "Namespace",
					DisplayName: "Cross Namespace User",
				},
			}
			if err := cfg.Client().Resources().Create(ctx, user); err != nil {
				t.Fatal(err)
			}

			// Wait for user to be ready
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(user, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDUser)
				if !ok {
					return false
				}
				return obj.Status.Ready && obj.Status.UserID != ""
			}), wait.WithTimeout(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Cleanup
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{Name: "cross-ns-user", Namespace: "default"},
			}
			cfg.Client().Resources().Delete(ctx, user)

			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "shared-instance", Namespace: "auth-system"},
			}
			cfg.Client().Resources().Delete(ctx, instance)

			authNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "auth-system"},
			}
			cfg.Client().Resources().Delete(ctx, authNs)

			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
