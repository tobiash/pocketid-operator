package e2e

import (
	"context"
	"testing"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestPocketIDInstance(t *testing.T) {
	feature := features.New("PocketIDInstance Lifecycle").
		Assess("Verify instance is ready and initialized", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: ns,
				},
			}
			if err := cfg.Client().Resources().Get(ctx, instanceName, ns, instance); err != nil {
				t.Fatalf("failed to get instance: %v", err)
			}

			if !instance.Status.Ready {
				t.Fatalf("Instance is not ready")
			}
			t.Logf("Instance is ready")

			return ctx
		}).
		Assess("Verify initialization completed", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: ns,
				},
			}
			if err := cfg.Client().Resources().Get(ctx, instanceName, ns, instance); err != nil {
				t.Fatalf("failed to get instance: %v", err)
			}

			initialized := false
			for _, cond := range instance.Status.Conditions {
				if cond.Type == "Initialized" && cond.Status == metav1.ConditionTrue {
					initialized = true
					break
				}
			}
			if !initialized {
				t.Fatalf("Instance not initialized (admin user not created)")
			}
			t.Logf("Instance initialization complete")

			// Verify API key secret exists
			apiKeySecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-api-key",
					Namespace: ns,
				},
			}
			if err := cfg.Client().Resources().Get(ctx, apiKeySecret.Name, ns, apiKeySecret); err != nil {
				t.Fatalf("failed to get API key secret: %v", err)
			}
			if _, ok := apiKeySecret.Data["STATIC_API_KEY"]; !ok {
				t.Fatalf("API key secret missing STATIC_API_KEY")
			}
			t.Logf("API key secret exists")

			// Verify service exists
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-svc",
					Namespace: ns,
				},
			}
			if err := cfg.Client().Resources().Get(ctx, svc.Name, ns, svc); err != nil {
				t.Fatalf("failed to get service: %v", err)
			}
			t.Logf("Service exists: %s", svc.Name)

			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
