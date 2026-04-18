package e2e

import (
	"context"
	"testing"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestOIDCClient(t *testing.T) {
	feature := features.New("OIDC Client Lifecycle").
		Assess("Create OIDC Client", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			client := &pocketidv1alpha1.PocketIDOIDCClient{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-client",
					Namespace: ns,
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
				t.Fatalf("failed to create OIDC client: %v", err)
			}

			t.Logf("OIDC client %s created in namespace %s", client.Name, ns)
			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
