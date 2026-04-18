package e2e

import (
	"context"
	"testing"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestUserAndGroupLifecycle(t *testing.T) {
	feature := features.New("User and Group Lifecycle").
		Assess("Create User Group", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			group := &pocketidv1alpha1.PocketIDUserGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-group",
					Namespace: ns,
				},
				Spec: pocketidv1alpha1.PocketIDUserGroupSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: "test-instance",
					},
					Name:         "developers",
					FriendlyName: "Development Team",
				},
			}
			if err := cfg.Client().Resources().Create(ctx, group); err != nil {
				t.Fatalf("failed to create user group: %v", err)
			}

			t.Logf("User group %s created in namespace %s", group.Name, ns)
			return ctx
		}).
		Assess("Create User", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			email := "user1@test.com"
			user := &pocketidv1alpha1.PocketIDUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: ns,
				},
				Spec: pocketidv1alpha1.PocketIDUserSpec{
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: "test-instance",
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

			t.Logf("User %s created in namespace %s", user.Name, ns)
			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
