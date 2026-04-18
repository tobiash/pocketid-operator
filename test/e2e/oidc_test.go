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

func TestOIDCClient(t *testing.T) {
	feature := features.New("OIDC Client Lifecycle").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create PocketID Instance
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "http://pocketid.test",
				},
			}
			if err := cfg.Client().Resources().Create(ctx, instance); err != nil {
				t.Fatal(err)
			}

			// Wait for instance? The controller might take time to set status.
			// For OIDC client test, we just need the CR to exist so the OIDC controller can find it.
			// In a real E2E, we might want to wait for the instance to be actually ready (pods running).
			// But since we are testing OIDC logic which depends on the API mostly.
			// Wait, the OIDC controller talks to the PocketID API (the pod service).
			// So we MUST wait for the PocketID instance to be ready and reachable!

			// This implies the PocketID image must be working and reachable in the cluster.
			// The operator deploys the StatefulSet.

			// Monitor PocketIDInstance status for Ready
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
		Assess("Create OIDC Client", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := &pocketidv1alpha1.PocketIDOIDCClient{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-client",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
					Name: "E2E Test App",
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: "test-instance",
					},
					CallbackURLs: []string{"http://localhost/callback"},
					CredentialsSecretRef: &pocketidv1alpha1.LocalObjectReference{
						Name: "test-client-secret",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, client); err != nil {
				t.Fatal(err)
			}

			// Wait for Ready condition
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(client, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDOIDCClient)
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
				t.Fatal(err)
			}

			// Verify Secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "test-client-secret", Namespace: "default"},
			}
			if err := cfg.Client().Resources().Get(ctx, secret.Name, secret.Namespace, secret); err != nil {
				t.Fatal(err)
			}
			if len(secret.Data["OIDC_CLIENT_ID"]) == 0 {
				t.Error("Secret OIDC_CLIENT_ID is empty")
			}
			if len(secret.Data["OIDC_CLIENT_SECRET"]) == 0 {
				t.Error("Secret OIDC_CLIENT_SECRET is empty")
			}

			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
