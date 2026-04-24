package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var (
	testEnv       env.Environment
	testNamespace = os.Getenv("TEST_NAMESPACE")
	clusterName   = os.Getenv("KIND_CLUSTER_NAME")
	instanceName  = "test-instance"
)

func init() {
	if testNamespace == "" {
		testNamespace = "pocketid-e2e"
	}
	if clusterName == "" {
		clusterName = "pocketid-test"
	}
}

func TestMain(m *testing.M) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = conf.ResolveKubeConfigFile()
	}

	cfg := envconf.NewWithKubeConfig(kubeconfig)

	testEnv = env.NewWithConfig(cfg)

	testEnv.Setup(
		envfuncs.CreateNamespace(testNamespace),
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			if err := pocketidv1alpha1.AddToScheme(cfg.Client().Resources().GetScheme()); err != nil {
				return ctx, err
			}
			return ctx, nil
		},
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			if err := gatewayv1.Install(cfg.Client().Resources().GetScheme()); err != nil {
				return ctx, err
			}
			return ctx, nil
		},
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "http://test-instance-svc." + testNamespace + ".svc.cluster.local",
					InitialAdmin: &pocketidv1alpha1.InitialAdminConfig{
						Email:       "admin@test.com",
						Username:    "admin",
						FirstName:   "Admin",
						DisplayName: "Admin User",
					},
				},
			}
			if err := cfg.Client().Resources().Create(ctx, instance); err != nil {
				return ctx, err
			}

			err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(instance, func(object k8s.Object) bool {
				obj, ok := object.(*pocketidv1alpha1.PocketIDInstance)
				if !ok {
					return false
				}
				return obj.Status.Ready
			}), wait.WithTimeout(5*time.Minute))
			return ctx, err
		},
	)

	testEnv.Finish(
		envfuncs.DeleteNamespace(testNamespace),
	)

	os.Exit(testEnv.Run(m))
}
