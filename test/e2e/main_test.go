package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	namespace       = "pocketid-e2e"
)

func init() {
	if kindClusterName == "" {
		kindClusterName = "pocketid-e2e"
	}
}

func TestMain(m *testing.M) {
	cfg, err := envconf.NewFromFlags()
	if err != nil {
		// fallback to default if flags are not provided (e.g. running via IDE)
		cfg = envconf.NewWithKubeConfig(conf.ResolveKubeConfigFile())
	}

	testEnv = env.NewWithConfig(cfg)

	// Create Kind cluster if not skipped
	if os.Getenv("SKIP_KIND_CREATION") != "true" {
		testEnv.Setup(
			envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
			envfuncs.CreateNamespace(namespace),
			func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
				if err := pocketidv1alpha1.AddToScheme(cfg.Client().Resources().GetScheme()); err != nil {
					return ctx, err
				}
				return ctx, nil
			},
			envfuncs.LoadDockerImageToCluster(kindClusterName, "controller:latest"),
			// Deploy operator
			func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
				// Set image in kustomize
				setImage := exec.Command("../../bin/kustomize", "edit", "set", "image", "controller=controller:latest")
				setImage.Dir = "../../config/manager"
				if out, err := setImage.CombinedOutput(); err != nil {
					return ctx, fmt.Errorf("failed to set image: %s: %w", out, err)
				}

				// Build manifests
				build := exec.Command("../../bin/kustomize", "build", "../../config/default")
				manifests, err := build.Output()
				if err != nil {
					return ctx, fmt.Errorf("failed to build manifests: %w", err)
				}

				// Apply manifests
				apply := exec.Command("kubectl", "apply", "-f", "-")
				apply.Env = os.Environ()
				if cfg.KubeconfigFile() != "" {
					apply.Env = append(apply.Env, "KUBECONFIG="+cfg.KubeconfigFile())
				}

				stdin, err := apply.StdinPipe()
				if err != nil {
					return ctx, err
				}

				go func() {
					defer stdin.Close()
					stdin.Write(manifests)
				}()

				if out, err := apply.CombinedOutput(); err != nil {
					return ctx, fmt.Errorf("failed to deploy: %s: %w", out, err)
				}

				return ctx, nil
			},
		)

		testEnv.Finish(
			envfuncs.DeleteNamespace(namespace),
			envfuncs.DestroyCluster(kindClusterName),
		)
	} else {
		// If skipping creation, we assume cluster exists, just create/delete namespace
		testEnv.Setup(
			envfuncs.CreateNamespace(namespace),
			func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
				if err := pocketidv1alpha1.AddToScheme(cfg.Client().Resources().GetScheme()); err != nil {
					return ctx, err
				}
				return ctx, nil
			},
		)
		testEnv.Finish(
			envfuncs.DeleteNamespace(namespace),
		)
	}

	os.Exit(testEnv.Run(m))
}
