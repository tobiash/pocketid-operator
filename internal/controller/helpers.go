package controller

import (
	"context"
	"errors"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	"github.com/tobiash/pocketid-operator/internal/pocketid"
)

var ErrAPIKeyNotFound = errors.New("API key not found in secret")

// getSecret retrieves a secret from the cluster
func getSecret(ctx context.Context, c client.Client, name, namespace string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", namespace, name, err)
	}
	return secret, nil
}

// createAPIClient creates a PocketID API client for an instance
func createAPIClient(ctx context.Context, c client.Client, instance *pocketidv1alpha1.PocketIDInstance) (*pocketid.Client, error) {
	return createAPIClientWithDevDefault(ctx, c, instance, "")
}

func createAPIClientWithDevDefault(ctx context.Context, c client.Client, instance *pocketidv1alpha1.PocketIDInstance, defaultDevURL string) (*pocketid.Client, error) {
	// Get the static API key secret
	apiKeySecretName := instance.Name + "-api-key"
	if instance.Spec.StaticAPIKeySecretRef != nil {
		apiKeySecretName = instance.Spec.StaticAPIKeySecretRef.Name
	}

	secret, err := getSecret(ctx, c, apiKeySecretName, instance.Namespace)
	if err != nil {
		return nil, err
	}

	apiKey := string(secret.Data["STATIC_API_KEY"])
	if apiKey == "" {
		return nil, ErrAPIKeyNotFound
	}

	baseURL := instance.Status.InternalURL
	if baseURL == "" {
		baseURL = pocketIDInternalURL(instance.Name, instance.Namespace)
	}

	if os.Getenv("KUBERNETES_SERVICE_HOST") == "" {
		if devURL := os.Getenv("POCKETID_DEV_API_URL"); devURL != "" {
			baseURL = devURL
		} else if defaultDevURL != "" {
			baseURL = defaultDevURL
		}
	}

	return pocketid.NewClient(baseURL, apiKey), nil
}

func pocketIDInternalURL(name, namespace string) string {
	return fmt.Sprintf("http://%s-svc.%s.svc.cluster.local", name, namespace)
}
