package e2e

import (
	"context"
	"os"
	"testing"

	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var (
	testEnv       env.Environment
	testNamespace = os.Getenv("TEST_NAMESPACE")
	clusterName   = os.Getenv("KIND_CLUSTER_NAME")
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
	)

	testEnv.Finish(
		envfuncs.DeleteNamespace(testNamespace),
	)

	os.Exit(testEnv.Run(m))
}
