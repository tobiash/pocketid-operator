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
			return ctx
		}).
		Assess("Create OIDC client and wait for ready", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			client := &pocketidv1alpha1.PocketIDOIDCClient{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-oidc-client",
					Namespace: ns,
				},
				Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
					Name: "E2E Test App",
					InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
						Name: instanceName,
					},
					CallbackURLs: []string{"http://localhost/callback"},
					CredentialsSecretRef: &pocketidv1alpha1.LocalObjectReference{
						Name: "test-oidc-credentials",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, client); err != nil {
				t.Fatalf("failed to create OIDC client: %v", err)
			}
			t.Logf("Created OIDC client")

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
				dumpClientStatus(ctx, t, cfg, ns, "test-oidc-client")
				t.Fatalf("OIDC client did not become ready: %v", err)
			}
			t.Logf("OIDC client is ready")

			// Verify status
			if err := cfg.Client().Resources().Get(ctx, "test-oidc-client", ns, client); err != nil {
				t.Fatalf("failed to get client: %v", err)
			}
			if client.Status.ClientID == "" {
				t.Fatalf("ClientID not set")
			}
			t.Logf("ClientID: %s", client.Status.ClientID)

			return ctx
		}).
		Assess("Verify credentials secret", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-oidc-credentials",
					Namespace: ns,
				},
			}
			if err := cfg.Client().Resources().Get(ctx, secret.Name, ns, secret); err != nil {
				t.Fatalf("failed to get secret: %v", err)
			}

			if _, ok := secret.Data["OIDC_CLIENT_ID"]; !ok {
				t.Fatalf("Secret missing OIDC_CLIENT_ID")
			}
			if _, ok := secret.Data["OIDC_CLIENT_SECRET"]; !ok {
				t.Fatalf("Secret missing OIDC_CLIENT_SECRET")
			}
			t.Logf("Credentials secret verified")

			return ctx
		}).
		Assess("Update OIDC client", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			client := &pocketidv1alpha1.PocketIDOIDCClient{
				ObjectMeta: metav1.ObjectMeta{Name: "test-oidc-client", Namespace: ns},
			}
			if err := cfg.Client().Resources().Get(ctx, client.Name, ns, client); err != nil {
				t.Fatalf("failed to get client: %v", err)
			}

			client.Spec.CallbackURLs = []string{"http://localhost/callback", "http://localhost/callback2"}
			if err := cfg.Client().Resources().Update(ctx, client); err != nil {
				t.Fatalf("failed to update client: %v", err)
			}
			t.Logf("Updated OIDC client with new callback URL")

			// Wait for sync
			time.Sleep(5 * time.Second)

			return ctx
		}).
		Assess("Delete OIDC client", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			client := &pocketidv1alpha1.PocketIDOIDCClient{
				ObjectMeta: metav1.ObjectMeta{Name: "test-oidc-client", Namespace: ns},
			}
			if err := cfg.Client().Resources().Delete(ctx, client); err != nil {
				t.Fatalf("failed to delete client: %v", err)
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceDeleted(client), wait.WithTimeout(1*time.Minute))
			if err != nil {
				t.Fatalf("client not deleted: %v", err)
			}
			t.Logf("Client deleted")

			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}

func dumpClientStatus(ctx context.Context, t *testing.T, cfg *envconf.Config, ns, name string) {
	client := &pocketidv1alpha1.PocketIDOIDCClient{}
	if err := cfg.Client().Resources().Get(ctx, name, ns, client); err == nil {
		t.Logf("Client status: %+v", client.Status)
	}
}
