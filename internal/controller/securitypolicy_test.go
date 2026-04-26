package controller

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var _ = Describe("SecurityPolicy Builder", func() {
	It("should build a SecurityPolicy with correct OIDC config", func() {
		oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app",
				Namespace: "default",
			},
			Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
				Name: "my-app",
				EnvoyGateway: &pocketidv1alpha1.EnvoyGatewayConfig{
					Enabled: true,
					HTTPRouteRef: &pocketidv1alpha1.NamespacedObjectReference{
						Name:      "my-app-route",
						Namespace: "default",
					},
				},
			},
			Status: pocketidv1alpha1.PocketIDOIDCClientStatus{
				ClientID:              "client-123",
				CredentialsSecretName: "my-app-credentials",
			},
		}

		instance := &pocketidv1alpha1.PocketIDInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pocket-id",
				Namespace: "auth",
			},
			Spec: pocketidv1alpha1.PocketIDInstanceSpec{
				AppURL: "https://auth.example.com",
			},
		}

		route := &gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-app-route",
				Namespace: "default",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				Hostnames: []gatewayv1.Hostname{"myapp.example.com"},
			},
		}

		sp, err := buildSecurityPolicy(oidcClient, instance, route, "my-app-credentials", scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		Expect(sp.GetName()).To(Equal("my-app"))
		Expect(sp.GetNamespace()).To(Equal("default"))
		Expect(sp.GetKind()).To(Equal("SecurityPolicy"))
		Expect(sp.GroupVersionKind().Group).To(Equal("gateway.envoyproxy.io"))

		spec := sp.Object["spec"].(map[string]any)
		targetRefs := spec["targetRefs"].([]any)
		Expect(targetRefs).To(HaveLen(1))
		ref := targetRefs[0].(map[string]any)
		Expect(ref["group"]).To(Equal("gateway.networking.k8s.io"))
		Expect(ref["kind"]).To(Equal("HTTPRoute"))
		Expect(ref["name"]).To(Equal("my-app-route"))

		oidc := spec["oidc"].(map[string]any)
		Expect(oidc["clientID"]).To(Equal("client-123"))
		Expect(oidc["redirectURL"]).To(Equal("https://myapp.example.com/oauth2/callback"))
		Expect(oidc["logoutPath"]).To(Equal("/logout"))

		provider := oidc["provider"].(map[string]any)
		Expect(provider["issuer"]).To(Equal("https://auth.example.com"))
		Expect(provider["authorizationEndpoint"]).To(Equal("https://auth.example.com/authorize"))
		Expect(provider["tokenEndpoint"]).To(Equal("https://auth.example.com/token"))

		secret := oidc["clientSecret"].(map[string]any)
		Expect(secret["name"]).To(Equal("my-app-credentials"))

		ownerRefs := sp.GetOwnerReferences()
		Expect(ownerRefs).To(HaveLen(1))
		Expect(ownerRefs[0].Name).To(Equal("my-app"))
		Expect(ownerRefs[0].Kind).To(Equal("PocketIDOIDCClient"))
	})

	It("should use custom callback and logout paths", func() {
		oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "custom-app",
				Namespace: "default",
			},
			Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
				EnvoyGateway: &pocketidv1alpha1.EnvoyGatewayConfig{
					Enabled:      true,
					CallbackPath: "/auth/callback",
					LogoutPath:   "/auth/logout",
					HTTPRouteRef: &pocketidv1alpha1.NamespacedObjectReference{
						Name: "custom-route",
					},
				},
			},
			Status: pocketidv1alpha1.PocketIDOIDCClientStatus{
				ClientID:              "client-456",
				CredentialsSecretName: "custom-creds",
			},
		}

		instance := &pocketidv1alpha1.PocketIDInstance{
			Spec: pocketidv1alpha1.PocketIDInstanceSpec{
				AppURL: "https://auth.example.com",
			},
		}

		route := &gatewayv1.HTTPRoute{
			Spec: gatewayv1.HTTPRouteSpec{
				Hostnames: []gatewayv1.Hostname{"custom.example.com"},
			},
		}

		sp, err := buildSecurityPolicy(oidcClient, instance, route, "custom-creds", scheme.Scheme)
		Expect(err).NotTo(HaveOccurred())

		spec := sp.Object["spec"].(map[string]any)
		oidc := spec["oidc"].(map[string]any)
		Expect(oidc["redirectURL"]).To(Equal("https://custom.example.com/auth/callback"))
		Expect(oidc["logoutPath"]).To(Equal("/auth/logout"))
	})

	It("should return error when HTTPRoute has no hostnames", func() {
		oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "no-host",
				Namespace: "default",
			},
		}

		instance := &pocketidv1alpha1.PocketIDInstance{}
		route := &gatewayv1.HTTPRoute{}

		_, err := buildSecurityPolicy(oidcClient, instance, route, "creds", scheme.Scheme)
		Expect(err).To(MatchError(ContainSubstring("no hostnames")))
	})
})

func TestSecurityPolicyGVK(t *testing.T) {
	g := NewWithT(t)

	sp := &unstructured.Unstructured{}
	sp.SetGroupVersionKind(securityPolicyGVK)

	g.Expect(sp.GetKind()).To(Equal("SecurityPolicy"))
	g.Expect(sp.GroupVersionKind().Group).To(Equal("gateway.envoyproxy.io"))
	g.Expect(sp.GroupVersionKind().Version).To(Equal("v1alpha1"))
}
