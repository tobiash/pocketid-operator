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

var instanceName = "test-instance"

func TestPocketIDInstance(t *testing.T) {
	feature := features.New("PocketIDInstance Lifecycle").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: ns,
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "http://pocketid.test",
					InitialAdmin: &pocketidv1alpha1.InitialAdminConfig{
						Email:       "admin@test.com",
						Username:    "admin",
						FirstName:   "Admin",
						DisplayName: "Admin User",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, instance); err != nil {
				t.Fatalf("failed to create instance: %v", err)
			}
			t.Logf("Created PocketIDInstance %s in namespace %s", instanceName, ns)
			return ctx
		}).
		Assess("Wait for instance to be ready", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: ns,
				},
			}

			t.Log("Waiting for PocketIDInstance to become ready...")
			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(instance, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDInstance)
				if !ok {
					return false
				}
				t.Logf("Instance status: Ready=%v", obj.Status.Ready)
				return obj.Status.Ready
			}), wait.WithTimeout(5*time.Minute))
			if err != nil {
				t.Fatalf("PocketIDInstance did not become ready: %v", err)
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
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{Name: instanceName, Namespace: testNamespace},
			}
			cfg.Client().Resources().Delete(ctx, instance)
			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
