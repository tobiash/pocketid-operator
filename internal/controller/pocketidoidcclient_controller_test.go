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
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
)

var _ = Describe("PocketIDOIDCClient Controller", func() {
	var (
		ctx             context.Context
		cancel          context.CancelFunc
		clientName      string
		clientNs        string
		instanceName    string
		mockServer      *httptest.Server
		testReconciler  *PocketIDOIDCClientReconciler
		mgr             ctrl.Manager
		apiCalls        map[string]int
		createdClientID string
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		clientName = fmt.Sprintf("test-client-%d", time.Now().UnixNano())
		clientNs = "default"
		instanceName = "test-instance" // We don't need a real instance running, just the CR

		apiCalls = make(map[string]int)
		createdClientID = ""

		// Setup Mock Server
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			apiCalls[key]++

			// List Clients (Get All or Filter)
			if r.URL.Path == "/api/oidc/clients" && r.Method == "GET" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				if createdClientID != "" {
					fmt.Fprintf(w, `{"data": [{"id": "%s", "name": "%s", "clientId": "test-client-id"}]}`, createdClientID, clientName)
				} else {
					w.Write([]byte(`{"data": []}`))
				}
				return
			}

			// Get Single Client
			if r.Method == "GET" && len(r.URL.Path) > len("/api/oidc/clients/") {
				id := r.URL.Path[len("/api/oidc/clients/"):]
				if id == createdClientID {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					fmt.Fprintf(w, `{"id": "%s", "name": "%s", "clientId": "test-client-id"}`, createdClientID, clientName)
					return
				}
				w.WriteHeader(http.StatusNotFound)
				return
			}

			// Create Client
			if r.URL.Path == "/api/oidc/clients" && r.Method == "POST" {
				var body map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				createdClientID = "generated-uuid-123"
				w.WriteHeader(http.StatusCreated)
				fmt.Fprintf(w, `{"id": "%s", "name": "%s", "clientId": "test-client-id"}`, createdClientID, body["name"])
				return
			}

			// Generate Secret
			if r.Method == "POST" && len(r.URL.Path) > len("/api/oidc/clients/") && r.URL.Path[len(r.URL.Path)-7:] == "/secret" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"secret": "super-secret-value"}`))
				return
			}

			// Update Client
			if r.Method == "PUT" && len(r.URL.Path) > len("/api/oidc/clients/") {
				id := r.URL.Path[len("/api/oidc/clients/"):]
				if id == createdClientID {
					w.WriteHeader(http.StatusOK)
					fmt.Fprintf(w, `{"id": "%s", "name": "%s", "clientId": "test-client-id"}`, id, clientName)
					return
				}
				w.WriteHeader(http.StatusNotFound)
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))

		// Set Env Var for Controller to use Mock Server
		os.Setenv("POCKETID_DEV_API_URL", mockServer.URL)

		// Create dummy instance CR for reference
		instance := &pocketidv1alpha1.PocketIDInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: clientNs,
			},
			Spec: pocketidv1alpha1.PocketIDInstanceSpec{
				AppURL: "http://localhost",
			},
			Status: pocketidv1alpha1.PocketIDInstanceStatus{
				StaticAPIKeySecretName: "test-api-key", // Point to a dummy secret
			},
		}
		// Try to create, ignore if exists (from other tests)
		_ = k8sClient.Create(ctx, instance)

		// Create dummy API key secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-instance-api-key",
				Namespace: clientNs,
			},
			Data: map[string][]byte{
				"STATIC_API_KEY": []byte("test-key"),
			},
		}
		_ = k8sClient.Create(ctx, secret)

		// Setup Controller Manager
		var err error
		mgr, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme:                 scheme.Scheme,
			Metrics:                metrics.Options{BindAddress: "0"},
			HealthProbeBindAddress: "0",
			LeaderElection:         false,
		})
		Expect(err).NotTo(HaveOccurred())

		testReconciler = &PocketIDOIDCClientReconciler{
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
		mockServer.Close()
		os.Unsetenv("POCKETID_DEV_API_URL")

		// Cleanup
		client := &pocketidv1alpha1.PocketIDOIDCClient{}
		if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: clientName, Namespace: clientNs}, client); err == nil {
			k8sClient.Delete(context.Background(), client)
		}
	})

	It("should create OIDC Client and Secret", func() {
		secretName := "test-client-credentials"

		// Create OIDC Client
		oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: clientNs,
			},
			Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
				Name:         "My App",
				InstanceRef:  pocketidv1alpha1.CrossNamespaceObjectReference{Name: instanceName},
				CallbackURLs: []string{"http://localhost/callback"},
				CredentialsSecretRef: &pocketidv1alpha1.LocalObjectReference{
					Name: secretName,
				},
				IsPublic: false,
			},
		}
		Expect(k8sClient.Create(ctx, oidcClient)).To(Succeed())

		// Verify Status Update (ClientID set)
		Eventually(func() string {
			currClient := &pocketidv1alpha1.PocketIDOIDCClient{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: clientName, Namespace: clientNs}, currClient)
			if err != nil {
				return ""
			}
			return currClient.Status.ClientID
		}, time.Second*30, time.Millisecond*500).Should(Equal("generated-uuid-123"))

		// Verify Ready Condition
		Eventually(func() bool {
			currClient := &pocketidv1alpha1.PocketIDOIDCClient{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: clientName, Namespace: clientNs}, currClient)
			if err != nil {
				return false
			}
			return meta.IsStatusConditionTrue(currClient.Status.Conditions, "Ready")
		}, time.Second*30, time.Millisecond*500).Should(BeTrue())

		// Verify Secret Creation
		Eventually(func() error {
			secret := &corev1.Secret{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: clientNs}, secret)
			if err != nil {
				return err
			}
			if string(secret.Data["OIDC_CLIENT_ID"]) != "generated-uuid-123" {
				return fmt.Errorf("wrong client ID in secret")
			}
			if string(secret.Data["OIDC_CLIENT_SECRET"]) != "super-secret-value" {
				return fmt.Errorf("wrong client secret in secret")
			}
			return nil
		}, time.Second*30, time.Millisecond*500).Should(Succeed())

		// Verify API Calls were made
		Expect(apiCalls["POST /api/oidc/clients"]).To(BeNumerically(">=", 1))
		Expect(apiCalls["POST /api/oidc/clients/generated-uuid-123/secret"]).To(BeNumerically(">=", 1))
	})
})
