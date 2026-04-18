package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var _ = Describe("Cross-Namespace Reference Validation", func() {
	Context("When validating cross-namespace references", func() {
		var (
			instance *pocketidv1alpha1.PocketIDInstance
			ctx      context.Context
		)

		BeforeEach(func() {
			ctx = context.Background()
		})

		It("Should allow same-namespace references by default", func() {
			instance = &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
				},
			}

			allowed, reason, err := ValidateCrossNamespaceReference(ctx, k8sClient, instance, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
			Expect(reason).To(BeEmpty())
		})

		It("Should deny cross-namespace references when not configured", func() {
			instance = &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "auth-ns",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
				},
			}

			allowed, reason, err := ValidateCrossNamespaceReference(ctx, k8sClient, instance, "app-ns")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
			Expect(reason).To(ContainSubstring("does not allow cross-namespace references"))
		})

		It("Should allow cross-namespace references when From=All", func() {
			fromAll := pocketidv1alpha1.NamespacesFromAll
			instance = &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "auth-ns",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
					AllowedReferences: &pocketidv1alpha1.AllowedReferences{
						Namespaces: &pocketidv1alpha1.NamespacesFrom{
							From: &fromAll,
						},
					},
				},
			}

			allowed, reason, err := ValidateCrossNamespaceReference(ctx, k8sClient, instance, "app-ns")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
			Expect(reason).To(BeEmpty())
		})

		It("Should deny cross-namespace references when From=Same", func() {
			fromSame := pocketidv1alpha1.NamespacesFromSame
			instance = &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "auth-ns",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
					AllowedReferences: &pocketidv1alpha1.AllowedReferences{
						Namespaces: &pocketidv1alpha1.NamespacesFrom{
							From: &fromSame,
						},
					},
				},
			}

			allowed, reason, err := ValidateCrossNamespaceReference(ctx, k8sClient, instance, "app-ns")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
			Expect(reason).To(ContainSubstring("only allows same-namespace references"))
		})

		It("Should validate namespace selector", func() {
			// Create a namespace with matching label
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "prod-app",
					Labels: map[string]string{
						"env": "prod",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer k8sClient.Delete(ctx, ns)

			fromSelector := pocketidv1alpha1.NamespacesFromSelector
			instance = &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-instance",
					Namespace: "auth-ns",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
					AllowedReferences: &pocketidv1alpha1.AllowedReferences{
						Namespaces: &pocketidv1alpha1.NamespacesFrom{
							From: &fromSelector,
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"env": "prod",
								},
							},
						},
					},
				},
			}

			// Should allow namespace with matching label
			allowed, reason, err := ValidateCrossNamespaceReference(ctx, k8sClient, instance, "prod-app")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeTrue())
			Expect(reason).To(BeEmpty())

			// Should deny namespace without matching label
			allowed, reason, err = ValidateCrossNamespaceReference(ctx, k8sClient, instance, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(allowed).To(BeFalse())
			Expect(reason).To(ContainSubstring("does not match selector"))
		})
	})

	Context("When resolving instance references", func() {
		It("Should resolve same-namespace reference", func() {
			ctx := context.Background()
			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "local-instance",
					Namespace: "default",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
				},
			}
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())
			defer k8sClient.Delete(ctx, instance)

			ref := pocketidv1alpha1.CrossNamespaceObjectReference{
				Name: "local-instance",
			}

			resolved, err := ResolveInstanceReference(ctx, k8sClient, ref, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Name).To(Equal("local-instance"))
			Expect(resolved.Namespace).To(Equal("default"))
		})

		It("Should resolve cross-namespace reference", func() {
			ctx := context.Background()

			// Create namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-ns",
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer k8sClient.Delete(ctx, ns)

			instance := &pocketidv1alpha1.PocketIDInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "remote-instance",
					Namespace: "other-ns",
				},
				Spec: pocketidv1alpha1.PocketIDInstanceSpec{
					AppURL: "https://auth.example.com",
				},
			}
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())
			defer k8sClient.Delete(ctx, instance)

			otherNs := "other-ns"
			ref := pocketidv1alpha1.CrossNamespaceObjectReference{
				Name:      "remote-instance",
				Namespace: &otherNs,
			}

			resolved, err := ResolveInstanceReference(ctx, k8sClient, ref, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved.Name).To(Equal("remote-instance"))
			Expect(resolved.Namespace).To(Equal("other-ns"))
		})
	})
})
