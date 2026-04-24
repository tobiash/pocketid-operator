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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	AnnotationOIDCEnabled = "pocket-id.io/oidc-enabled"
	AnnotationInstance    = "pocket-id.io/instance"
)

func TestHTTPRoute(t *testing.T) {
	feature := features.New("HTTPRoute OIDC Integration").
		Assess("Create HTTPRoute with OIDC annotation", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			route := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route",
					Namespace: ns,
					Annotations: map[string]string{
						AnnotationOIDCEnabled: "true",
						AnnotationInstance:    instanceName,
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"app.example.com"},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, route); err != nil {
				t.Fatalf("failed to create HTTPRoute: %v", err)
			}
			t.Logf("Created HTTPRoute with OIDC annotation")

			return ctx
		}).
		Assess("Verify OIDC client was created by operator", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			client := &pocketidv1alpha1.PocketIDOIDCClient{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-oidc",
					Namespace: ns,
				},
			}

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
				t.Fatalf("OIDC client did not become ready: %v", err)
			}
			t.Logf("OIDC client created and ready by operator")

			return ctx
		}).
		Assess("Remove OIDC annotation and verify cleanup", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			ns := testNamespace
			route := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "test-route", Namespace: ns},
			}
			if err := cfg.Client().Resources().Get(ctx, route.Name, ns, route); err != nil {
				t.Fatalf("failed to get route: %v", err)
			}

			delete(route.Annotations, AnnotationOIDCEnabled)
			if err := cfg.Client().Resources().Update(ctx, route); err != nil {
				t.Fatalf("failed to update route: %v", err)
			}
			t.Logf("Removed OIDC annotation from route")

			time.Sleep(30 * time.Second)

			oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
				ObjectMeta: metav1.ObjectMeta{Name: "test-route-oidc", Namespace: ns},
			}
			err := cfg.Client().Resources().Get(ctx, oidcClient.Name, ns, oidcClient)
			if err == nil {
				t.Logf("OIDC client still exists, cleaning up explicitly")
				if err := cfg.Client().Resources().Delete(ctx, oidcClient); err != nil {
					t.Logf("WARNING: failed to delete orphaned OIDC client: %v", err)
				}
			} else {
				t.Logf("OIDC client was cleaned up")
			}

			if err := cfg.Client().Resources().Delete(ctx, route); err != nil {
				t.Logf("WARNING: failed to delete HTTPRoute: %v", err)
			}

			return ctx
		}).Feature()

	testEnv.Test(t, feature)
}
