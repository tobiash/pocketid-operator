package e2e

import (
	"context"
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var (
	testEnv         env.Environment
	kindClusterName = os.Getenv("KIND_CLUSTER_NAME")
	testNamespace   = os.Getenv("TEST_NAMESPACE")
)

func init() {
	if kindClusterName == "" {
		kindClusterName = "pocketid-test"
	}
	if testNamespace == "" {
		testNamespace = "pocketid-e2e"
	}
}

func TestMain(m *testing.M) {
	cfg, err := envconf.NewFromFlags()
	if err != nil {
		cfg = envconf.NewWithKubeConfig(conf.ResolveKubeConfigFile())
	}

	testEnv = env.NewWithConfig(cfg)

	if os.Getenv("SKIP_KIND_CREATION") != "true" {
		testEnv.Setup(
			envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
			envfuncs.CreateNamespace(testNamespace),
			func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
				if err := pocketidv1alpha1.AddToScheme(cfg.Client().Resources().GetScheme()); err != nil {
					return ctx, err
				}
				return ctx, nil
			},
		)

		testEnv.Finish(
			envfuncs.DeleteNamespace(testNamespace),
			envfuncs.DestroyCluster(kindClusterName),
		)
	} else {
		testEnv.Setup(
			envfuncs.CreateNamespace(testNamespace),
			func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
				if err := pocketidv1alpha1.AddToScheme(cfg.Client().Resources().GetScheme()); err != nil {
					return ctx, err
				}
				return ctx, nil
			},
		)
		testEnv.Finish(
			envfuncs.DeleteNamespace(testNamespace),
		)
	}

	os.Exit(testEnv.Run(m))
}
