package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var _ = Describe("HTTPRoute Controller", func() {
	const (
		PocketIDName      = "test-pocketid-route"
		PocketIDNamespace = "default"
		HTTPRouteName     = "test-route"
		OIDCClientName    = "test-route-oidc"
		Timeout           = time.Second * 10
		Interval          = time.Millisecond * 250
	)

	Context("When reconciling a HTTPRoute", Ordered, func() {
		var cancel context.CancelFunc

		BeforeAll(func() {
			var ctx context.Context
			ctx, cancel = context.WithCancel(context.Background())

			// Start the controller once for all tests
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:                 scheme.Scheme,
				Metrics:                metricsserver.Options{BindAddress: "0"},
				HealthProbeBindAddress: "0",
				LeaderElection:         false,
			})
			Expect(err).NotTo(HaveOccurred())

			testReconciler := &HTTPRouteReconciler{
				Client: mgr.GetClient(),
				Scheme: mgr.GetScheme(),
			}

			err = testReconciler.SetupWithManager(mgr)
			Expect(err).NotTo(HaveOccurred())

			go func() {
				defer GinkgoRecover()
				err = mgr.Start(ctx)
				Expect(err).NotTo(HaveOccurred())
			}()
		})

		AfterAll(func() {
			cancel()
		})

		It("Should create a PocketIDOIDCClient when enabled", func() {
			ctx := context.Background()

			// Create PocketIDInstance
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      PocketIDName,
					Namespace: PocketIDNamespace,
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
					InitialAdmin: &pocketidv1alpha1.InitialAdminConfig{
						Email:       "admin@example.com",
						Username:    "admin",
						FirstName:   "Admin",
						DisplayName: "Admin User",
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Create HTTPRoute
			route := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      HTTPRouteName,
					Namespace: PocketIDNamespace,
					Annotations: map[string]string{
						"pocket-id.io/oidc-enabled": "true",
						"pocket-id.io/instance":     PocketIDName,
						"pocket-id.io/client-name":  OIDCClientName,
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"app.example.com"},
				},
			}
			Expect(k8sClient.Create(ctx, route)).Should(Succeed())

			// Verify OIDCClient creation
			oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: OIDCClientName, Namespace: PocketIDNamespace}, oidcClient)
			}, Timeout, Interval).Should(Succeed())

			Expect(oidcClient.Spec.Name).To(Equal(OIDCClientName))
			Expect(oidcClient.Spec.InstanceRef.Name).To(Equal(PocketIDName))
			Expect(oidcClient.Spec.CallbackURLs).To(ContainElement("https://app.example.com/callback"))

			// Verify OwnerReference
			Expect(oidcClient.OwnerReferences).To(HaveLen(1))
			Expect(oidcClient.OwnerReferences[0].Name).To(Equal(HTTPRouteName))
			Expect(oidcClient.OwnerReferences[0].Kind).To(Equal("HTTPRoute"))
		})

		It("Should update PocketIDOIDCClient when HTTPRoute changes", func() {
			ctx := context.Background()

			// Create a separate PocketIDInstance for this test
			instance2 := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pocketid-route-2",
					Namespace: PocketIDNamespace,
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth2.example.com",
					InitialAdmin: &pocketidv1alpha1.InitialAdminConfig{
						Email:       "admin2@example.com",
						Username:    "admin2",
						FirstName:   "Admin",
						DisplayName: "Admin User 2",
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance2)).Should(Succeed())

			// Create a new HTTPRoute for this test
			route2 := &gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-route-2",
					Namespace: PocketIDNamespace,
					Annotations: map[string]string{
						"pocket-id.io/oidc-enabled": "true",
						"pocket-id.io/instance":     "test-pocketid-route-2",
						"pocket-id.io/client-name":  "test-route-2-oidc",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					Hostnames: []gatewayv1.Hostname{"app2.example.com"},
				},
			}
			Expect(k8sClient.Create(ctx, route2)).Should(Succeed())

			// Wait for initial OIDC client creation
			oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "test-route-2-oidc", Namespace: PocketIDNamespace}, oidcClient)
			}, Timeout, Interval).Should(Succeed())

			// Update HTTPRoute hostname
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-route-2", Namespace: PocketIDNamespace}, route2)).Should(Succeed())
			route2.Spec.Hostnames = []gatewayv1.Hostname{"new-app2.example.com"}
			Expect(k8sClient.Update(ctx, route2)).Should(Succeed())

			// Verify OIDCClient update
			Eventually(func() []string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-route-2-oidc", Namespace: PocketIDNamespace}, oidcClient)
				if err != nil {
					return nil
				}
				return oidcClient.Spec.CallbackURLs
			}, Timeout, Interval).Should(ContainElement("https://new-app2.example.com/callback"))
		})
	})
})
