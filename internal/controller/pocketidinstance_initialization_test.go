/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var _ = Describe("PocketIDInstance Initialization", func() {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		instanceName   string
		instanceNs     string
		mockServer     *httptest.Server
		createdAdmin   bool
		testReconciler *PocketIDInstanceReconciler
		mgr            ctrl.Manager
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		instanceName = fmt.Sprintf("test-instance-%d", time.Now().UnixNano())
		instanceNs = "default"

		// Create a listener on port 1411 to intercept the controller's fallback request
		l, err := net.Listen("tcp", "127.0.0.1:1411")
		Expect(err).NotTo(HaveOccurred(), "Port 1411 must be free for this test to run")

		// Setup Mock Server using the custom listener
		createdAdmin = false
		mockServer = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/users" && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"data": []}`))
				return
			}

			if r.URL.Path == "/api/users" && r.Method == "POST" {
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if body["username"] == "admin" && body["isAdmin"] == true {
					createdAdmin = true
					w.WriteHeader(http.StatusCreated)
					w.Write([]byte(`{"id": 1, "username": "admin"}`))
				} else {
					w.WriteHeader(http.StatusBadRequest)
				}
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))

		// Close the listener created by httptest.NewUnstartedServer, and replace it with our custom listener
		mockServer.Listener.Close()
		mockServer.Listener = l
		mockServer.Start()

		// Setup Controller Manager
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme:                 scheme.Scheme,
			Metrics:                metrics.Options{BindAddress: "0"}, // Disable metrics
			HealthProbeBindAddress: "0",
			LeaderElection:         false,
		})
		Expect(err).NotTo(HaveOccurred())

		testReconciler = &PocketIDInstanceReconciler{
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

	AfterEach(func() {
		cancel()
		if mockServer != nil {
			mockServer.Close()
		}

		// Cleanup resources
		instance := &pocketidv1alpha1.PocketIDInstance{}
		err := k8sClient.Get(context.Background(), types.NamespacedName{Name: instanceName, Namespace: instanceNs}, instance)
		if err == nil {
			k8sClient.Delete(context.Background(), instance)
		}
	})

	It("should initialize PocketID with admin user when InitialAdmin is set", func() {
		// Define expected Secret names
		apiKeySecretName := fmt.Sprintf("%s-api-key", instanceName)

		// Create PocketIDInstance
		instance := &pocketidv1alpha1.PocketIDInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: instanceNs,
			},
			Spec: pocketidv1alpha1.PocketIDInstanceSpec{
				AppURL: "http://localhost",
				InitialAdmin: &pocketidv1alpha1.InitialAdminConfig{
					Username:    "admin",
					Email:       "admin@example.com",
					FirstName:   "Admin",
					DisplayName: "Administrator",
				},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).To(Succeed())

		// Wait for StatefulSet to be created
		stsDetails := types.NamespacedName{Name: instanceName, Namespace: instanceNs}
		Eventually(func() error {
			sts := &appsv1.StatefulSet{}
			return k8sClient.Get(ctx, stsDetails, sts)
		}, time.Second*10, time.Millisecond*250).Should(Succeed())

		// Simulate Pod Readiness
		// We need to update the Status of the StatefulSet because there is no controller doing it in envtest
		sts := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(ctx, stsDetails, sts)).To(Succeed())

		replicas := int32(1)
		sts.Status.Replicas = replicas
		sts.Status.ReadyReplicas = replicas
		sts.Status.CurrentReplicas = replicas
		sts.Status.UpdatedReplicas = replicas

		Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

		// Wait for Initialization
		Eventually(func() bool {
			currInstance := &pocketidv1alpha1.PocketIDInstance{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: instanceName, Namespace: instanceNs}, currInstance)
			if err != nil {
				return false
			}

			// Check if Initialized condition is True
			cond := meta.FindStatusCondition(currInstance.Status.Conditions, "Initialized")
			if cond != nil && cond.Status == metav1.ConditionTrue {
				return true
			}
			return false
		}, time.Second*30, time.Millisecond*500).Should(BeTrue(), "Instance should have Initialized condition = True")

		// Verify that admin was created
		Expect(createdAdmin).To(BeTrue(), "Admin user creation endpoint should have been called")

		// Verify Secret existence
		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: apiKeySecretName, Namespace: instanceNs}, secret)).To(Succeed())
		Expect(secret.Data).To(HaveKey("STATIC_API_KEY"))
	})
})
