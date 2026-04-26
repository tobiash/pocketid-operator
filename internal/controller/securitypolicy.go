package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var securityPolicyGVR = schema.GroupVersionResource{
	Group:    "gateway.envoyproxy.io",
	Version:  "v1alpha1",
	Resource: "securitypolicies",
}

var securityPolicyGVK = schema.GroupVersionKind{
	Group:   "gateway.envoyproxy.io",
	Version: "v1alpha1",
	Kind:    "SecurityPolicy",
}

func buildSecurityPolicy(
	oidcClient *pocketidv1alpha1.PocketIDOIDCClient,
	instance *pocketidv1alpha1.PocketIDInstance,
	route *gatewayv1.HTTPRoute,
	credentialsSecretName string,
	scheme *runtime.Scheme,
) (*unstructured.Unstructured, error) {
	if len(route.Spec.Hostnames) == 0 {
		return nil, fmt.Errorf("HTTPRoute %s/%s has no hostnames", route.Namespace, route.Name)
	}

	eg := oidcClient.Spec.EnvoyGateway
	callbackPath := "/oauth2/callback"
	if eg.CallbackPath != "" {
		callbackPath = eg.CallbackPath
	}
	logoutPath := "/logout"
	if eg.LogoutPath != "" {
		logoutPath = eg.LogoutPath
	}

	hostname := string(route.Spec.Hostnames[0])
	redirectURL := fmt.Sprintf("https://%s%s", hostname, callbackPath)

	appURL := instance.Spec.AppURL

	sp := &unstructured.Unstructured{}
	sp.SetGroupVersionKind(securityPolicyGVK)
	sp.SetName(oidcClient.Name)
	sp.SetNamespace(oidcClient.Namespace)

	sp.Object["spec"] = map[string]any{
		"targetRefs": []any{
			map[string]any{
				"group": "gateway.networking.k8s.io",
				"kind":  "HTTPRoute",
				"name":  route.Name,
			},
		},
		"oidc": map[string]any{
			"provider": map[string]any{
				"issuer":                appURL,
				"authorizationEndpoint": appURL + "/authorize",
				"tokenEndpoint":         appURL + "/token",
			},
			"clientID": oidcClient.Status.ClientID,
			"clientSecret": map[string]any{
				"name": credentialsSecretName,
			},
			"redirectURL": redirectURL,
			"logoutPath":  logoutPath,
		},
	}

	if err := controllerutil.SetControllerReference(oidcClient, sp, scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return sp, nil
}

func securityPolicyKey(oidcClient *pocketidv1alpha1.PocketIDOIDCClient) client.ObjectKey {
	return client.ObjectKey{Name: oidcClient.Name, Namespace: oidcClient.Namespace}
}
