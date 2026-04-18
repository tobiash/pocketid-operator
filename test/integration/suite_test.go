package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pocketidv1alpha1 "github.com/tobiash/pocketid-operator/api/v1alpha1"
	"github.com/tobiash/pocketid-operator/internal/controller"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type MockServer struct {
	Server *httptest.Server
	URL    string

	mu          sync.RWMutex
	users       map[string]UserDto
	groups      map[string]UserGroupDto
	oidcClients map[string]OidcClientDto

	errors  map[string]error
	delays  map[string]time.Duration
	callLog []string
}

type UserDto struct {
	ID            string  `json:"id"`
	Username      string  `json:"username"`
	Email         *string `json:"email"`
	EmailVerified bool    `json:"emailVerified"`
	FirstName     string  `json:"firstName"`
	LastName      *string `json:"lastName"`
	DisplayName   string  `json:"displayName"`
	IsAdmin       bool    `json:"isAdmin"`
	Locale        *string `json:"locale"`
	Disabled      bool    `json:"disabled"`
}

type UserGroupDto struct {
	ID           string    `json:"id"`
	FriendlyName string    `json:"friendlyName"`
	Name         string    `json:"name"`
	UserCount    int64     `json:"userCount"`
	CreatedAt    time.Time `json:"createdAt"`
}

type UserGroupMinimalDto struct {
	ID           string `json:"id"`
	FriendlyName string `json:"friendlyName"`
	Name         string `json:"name"`
	UserCount    int64  `json:"userCount"`
}

type OidcClientMetaDataDto struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	RequiresReauthentication bool   `json:"requiresReauthentication"`
}

type OidcClientDto struct {
	OidcClientMetaDataDto
	CallbackURLs       []string `json:"callbackURLs"`
	LogoutCallbackURLs []string `json:"logoutCallbackURLs"`
	IsPublic           bool     `json:"isPublic"`
	PkceEnabled        bool     `json:"pkceEnabled"`
	IsGroupRestricted  bool     `json:"isGroupRestricted"`
}

type OidcClientCreateDto struct {
	OidcClientDto
	ID string `json:"id"`
}

type OidcClientWithAllowedUserGroupsDto struct {
	OidcClientDto
	AllowedUserGroups []UserGroupMinimalDto `json:"allowedUserGroups"`
}

type UserCreateDto struct {
	Username    string  `json:"username"`
	Email       *string `json:"email"`
	FirstName   string  `json:"firstName"`
	LastName    string  `json:"lastName"`
	DisplayName string  `json:"displayName"`
	IsAdmin     bool    `json:"isAdmin"`
	Disabled    bool    `json:"disabled"`
}

type UserGroupCreateDto struct {
	FriendlyName string `json:"friendlyName"`
	Name         string `json:"name"`
}

type PaginationDto struct {
	TotalPages   int `json:"totalPages"`
	TotalItems   int `json:"totalItems"`
	CurrentPage  int `json:"currentPage"`
	ItemsPerPage int `json:"itemsPerPage"`
}

type PaginatedUsers struct {
	Data       []UserDto     `json:"data"`
	Pagination PaginationDto `json:"pagination"`
}

type PaginatedUserGroups struct {
	Data       []UserGroupDto `json:"data"`
	Pagination PaginationDto  `json:"pagination"`
}

type PaginatedOIDCClients struct {
	Data       []OidcClientDto `json:"data"`
	Pagination PaginationDto   `json:"pagination"`
}

type SecretResponse struct {
	Secret string `json:"secret"`
}

func NewMockServer() *MockServer {
	m := &MockServer{
		users:       make(map[string]UserDto),
		groups:      make(map[string]UserGroupDto),
		oidcClients: make(map[string]OidcClientDto),
		errors:      make(map[string]error),
		delays:      make(map[string]time.Duration),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handler())
	m.Server = httptest.NewServer(mux)
	m.URL = m.Server.URL
	return m
}

func (m *MockServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users = make(map[string]UserDto)
	m.groups = make(map[string]UserGroupDto)
	m.oidcClients = make(map[string]OidcClientDto)
	m.errors = make(map[string]error)
	m.delays = make(map[string]time.Duration)
	m.callLog = nil
}

func (m *MockServer) SetError(operation string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[operation] = err
}

func (m *MockServer) SetDelay(operation string, delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delays[operation] = delay
}

func (m *MockServer) CallLog() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.callLog)
}

func (m *MockServer) Close() {
	m.Server.Close()
}

func (m *MockServer) generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *MockServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.callLog = append(m.callLog, fmt.Sprintf("%s %s", r.Method, r.URL.Path))
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/users":
			m.listUsers(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/api/users":
			m.createUser(w, r)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/users/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/users/")
			m.deleteUser(w, r, id)
		case r.Method == http.MethodGet && r.URL.Path == "/api/groups":
			m.listGroups(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/api/groups":
			m.createGroup(w, r)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/groups/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/groups/")
			m.deleteGroup(w, r, id)
		case r.Method == http.MethodGet && r.URL.Path == "/api/oidc/clients":
			m.listOIDCClients(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/api/oidc/clients":
			m.createOIDCClient(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/secret") && strings.Contains(r.URL.Path, "/oidc/clients/"):
			parts := strings.Split(r.URL.Path, "/")
			id := parts[len(parts)-2]
			m.generateClientSecret(w, r, id)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/oidc/clients/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/oidc/clients/")
			m.updateOIDCClient(w, r, id)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/oidc/clients/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/oidc/clients/")
			m.deleteOIDCClient(w, r, id)
		default:
			http.Error(w, fmt.Sprintf("unhandled: %s %s", r.Method, r.URL.Path), http.StatusNotFound)
		}
	}
}

func (m *MockServer) checkError(op string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err, ok := m.errors[op]; ok {
		return err
	}
	return nil
}

func (m *MockServer) checkDelay(op string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if delay, ok := m.delays[op]; ok {
		time.Sleep(delay)
	}
}

func (m *MockServer) listUsers(w http.ResponseWriter, r *http.Request) {
	m.checkDelay("ListUsers")
	if err := m.checkError("ListUsers"); err != nil {
		writeError(w, err)
		return
	}

	m.mu.RLock()
	users := make([]UserDto, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, u)
	}
	m.mu.RUnlock()

	json.NewEncoder(w).Encode(PaginatedUsers{
		Data: users,
		Pagination: PaginationDto{
			TotalPages:   1,
			TotalItems:   len(users),
			CurrentPage:  1,
			ItemsPerPage: 20,
		},
	})
}

func (m *MockServer) createUser(w http.ResponseWriter, r *http.Request) {
	m.checkDelay("CreateUser")
	if err := m.checkError("CreateUser"); err != nil {
		writeError(w, err)
		return
	}

	var input UserCreateDto
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := m.generateID()
	user := UserDto{
		ID:            id,
		Username:      input.Username,
		Email:         input.Email,
		EmailVerified: false,
		FirstName:     input.FirstName,
		LastName:      &input.LastName,
		DisplayName:   input.DisplayName,
		IsAdmin:       input.IsAdmin,
		Disabled:      input.Disabled,
	}

	m.mu.Lock()
	m.users[id] = user
	m.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (m *MockServer) deleteUser(w http.ResponseWriter, r *http.Request, id string) {
	m.checkDelay("DeleteUser")
	if err := m.checkError("DeleteUser"); err != nil {
		writeError(w, err)
		return
	}

	m.mu.Lock()
	delete(m.users, id)
	m.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (m *MockServer) listGroups(w http.ResponseWriter, r *http.Request) {
	m.checkDelay("ListGroups")
	if err := m.checkError("ListGroups"); err != nil {
		writeError(w, err)
		return
	}

	m.mu.RLock()
	groups := make([]UserGroupDto, 0, len(m.groups))
	for _, g := range m.groups {
		groups = append(groups, g)
	}
	m.mu.RUnlock()

	json.NewEncoder(w).Encode(PaginatedUserGroups{
		Data: groups,
		Pagination: PaginationDto{
			TotalPages:   1,
			TotalItems:   len(groups),
			CurrentPage:  1,
			ItemsPerPage: 20,
		},
	})
}

func (m *MockServer) createGroup(w http.ResponseWriter, r *http.Request) {
	m.checkDelay("CreateGroup")
	if err := m.checkError("CreateGroup"); err != nil {
		writeError(w, err)
		return
	}

	var input UserGroupCreateDto
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := m.generateID()
	group := UserGroupDto{
		ID:           id,
		FriendlyName: input.FriendlyName,
		Name:         input.Name,
		UserCount:    0,
		CreatedAt:    time.Now(),
	}

	m.mu.Lock()
	m.groups[id] = group
	m.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(group)
}

func (m *MockServer) deleteGroup(w http.ResponseWriter, r *http.Request, id string) {
	m.checkDelay("DeleteGroup")
	if err := m.checkError("DeleteGroup"); err != nil {
		writeError(w, err)
		return
	}

	m.mu.Lock()
	delete(m.groups, id)
	m.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func (m *MockServer) listOIDCClients(w http.ResponseWriter, r *http.Request) {
	m.checkDelay("ListOIDCClients")
	if err := m.checkError("ListOIDCClients"); err != nil {
		writeError(w, err)
		return
	}

	m.mu.RLock()
	clients := make([]OidcClientDto, 0, len(m.oidcClients))
	for _, c := range m.oidcClients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	json.NewEncoder(w).Encode(PaginatedOIDCClients{
		Data: clients,
		Pagination: PaginationDto{
			TotalPages:   1,
			TotalItems:   len(clients),
			CurrentPage:  1,
			ItemsPerPage: 20,
		},
	})
}

func (m *MockServer) createOIDCClient(w http.ResponseWriter, r *http.Request) {
	m.checkDelay("CreateOIDCClient")
	if err := m.checkError("CreateOIDCClient"); err != nil {
		writeError(w, err)
		return
	}

	var input OidcClientCreateDto
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := input.ID
	if id == "" {
		id = m.generateID()
	}

	client := OidcClientDto{
		OidcClientMetaDataDto: OidcClientMetaDataDto{
			ID:                       id,
			Name:                     input.Name,
			RequiresReauthentication: input.RequiresReauthentication,
		},
		CallbackURLs:       input.CallbackURLs,
		LogoutCallbackURLs: input.LogoutCallbackURLs,
		IsPublic:           input.IsPublic,
		PkceEnabled:        input.PkceEnabled,
		IsGroupRestricted:  input.IsGroupRestricted,
	}

	m.mu.Lock()
	m.oidcClients[id] = client
	m.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(OidcClientWithAllowedUserGroupsDto{
		OidcClientDto: client,
	})
}

func (m *MockServer) generateClientSecret(w http.ResponseWriter, r *http.Request, id string) {
	m.checkDelay("GenerateClientSecret")
	if err := m.checkError("GenerateClientSecret"); err != nil {
		writeError(w, err)
		return
	}

	m.mu.RLock()
	_, ok := m.oidcClients[id]
	m.mu.RUnlock()

	if !ok {
		http.Error(w, "client not found", http.StatusNotFound)
		return
	}

	secret := m.generateID()
	json.NewEncoder(w).Encode(SecretResponse{Secret: secret})
}

func (m *MockServer) updateOIDCClient(w http.ResponseWriter, r *http.Request, id string) {
	m.checkDelay("UpdateOIDCClient")
	if err := m.checkError("UpdateOIDCClient"); err != nil {
		writeError(w, err)
		return
	}

	var input OidcClientCreateDto
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.oidcClients[id]; !ok {
		http.Error(w, "client not found", http.StatusNotFound)
		return
	}

	m.oidcClients[id] = OidcClientDto{
		OidcClientMetaDataDto: OidcClientMetaDataDto{
			ID:                       id,
			Name:                     input.Name,
			RequiresReauthentication: input.RequiresReauthentication,
		},
		CallbackURLs:       input.CallbackURLs,
		LogoutCallbackURLs: input.LogoutCallbackURLs,
		IsPublic:           input.IsPublic,
		PkceEnabled:        input.PkceEnabled,
		IsGroupRestricted:  input.IsGroupRestricted,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(OidcClientWithAllowedUserGroupsDto{
		OidcClientDto: m.oidcClients[id],
	})
}

func (m *MockServer) deleteOIDCClient(w http.ResponseWriter, r *http.Request, id string) {
	m.checkDelay("DeleteOIDCClient")
	if err := m.checkError("DeleteOIDCClient"); err != nil {
		writeError(w, err)
		return
	}

	m.mu.Lock()
	delete(m.oidcClients, id)
	m.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

func writeError(w http.ResponseWriter, err error) {
	var status int
	var msg string
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
		msg = "request timed out"
	} else {
		status = http.StatusInternalServerError
		msg = err.Error()
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

var (
	testEnv       *envtest.Environment
	k8sClient     client.Client
	mockServer    *MockServer
	ctx           context.Context
	cancel        context.CancelFunc
	controllerMgr ctrl.Manager
	testClient    client.Client // Client used by controllers in tests
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Test Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(30 * time.Second)
	SetDefaultEventuallyPollingInterval(500 * time.Millisecond)

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	envtestBin := filepath.Join("..", "..", "bin", "k8s", "1.29.0-linux-amd64")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases"), filepath.Join("..", "..", "bin", "gateway-api-crds")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: envtestBin,
	}

	var err error
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = pocketidv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = gatewayv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	By("Setting up mock PocketID server")
	mockServer = NewMockServer()
	os.Setenv("KUBERNETES_SERVICE_HOST", "")
	os.Setenv("POCKETID_DEV_API_URL", mockServer.URL)

	By("Setting up controller manager")
	controllerMgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		Metrics:                metrics.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		LeaderElection:         false,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&controller.PocketIDOIDCClientReconciler{
		Client: controllerMgr.GetClient(),
		Scheme: controllerMgr.GetScheme(),
	}).SetupWithManager(controllerMgr)
	Expect(err).NotTo(HaveOccurred())

	testClient = controllerMgr.GetClient()

	err = (&controller.PocketIDUserReconciler{
		Client: controllerMgr.GetClient(),
		Scheme: controllerMgr.GetScheme(),
	}).SetupWithManager(controllerMgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&controller.PocketIDUserGroupReconciler{
		Client: controllerMgr.GetClient(),
		Scheme: controllerMgr.GetScheme(),
	}).SetupWithManager(controllerMgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&controller.PocketIDInstanceReconciler{
		Client: controllerMgr.GetClient(),
		Scheme: controllerMgr.GetScheme(),
	}).SetupWithManager(controllerMgr)
	Expect(err).NotTo(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = controllerMgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	mockServer.Close()
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("POCKETID_DEV_API_URL")
	testEnv.Stop()
})

var _ = BeforeEach(func() {
	mockServer.Reset()
})

func createTestInstance(name, namespace string) *pocketidv1alpha1.PocketIDInstance {
	instance := &pocketidv1alpha1.PocketIDInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: pocketidv1alpha1.PocketIDInstanceSpec{
			AppURL: "https://auth.example.com",
		},
		Status: pocketidv1alpha1.PocketIDInstanceStatus{
			StaticAPIKeySecretName: name + "-api-key",
		},
	}
	err := k8sClient.Create(ctx, instance)
	Expect(err).NotTo(HaveOccurred())
	return instance
}

func createAPIKeySecret(name, namespace, apiKey string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"STATIC_API_KEY": []byte(apiKey),
		},
	}
	err := k8sClient.Create(ctx, secret)
	Expect(err).NotTo(HaveOccurred())
	return secret
}

func reconcileUser(name, namespace string) (ctrl.Result, error) {
	r := &controller.PocketIDUserReconciler{
		Client: testClient,
		Scheme: testClient.Scheme(),
	}
	return r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
}

func reconcileUserGroup(name, namespace string) (ctrl.Result, error) {
	r := &controller.PocketIDUserGroupReconciler{
		Client: testClient,
		Scheme: testClient.Scheme(),
	}
	return r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
}

func reconcileOIDCClient(name, namespace string) (ctrl.Result, error) {
	r := &controller.PocketIDOIDCClientReconciler{
		Client: testClient,
		Scheme: testClient.Scheme(),
	}
	return r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
}

func reconcileInstance(name, namespace string) (ctrl.Result, error) {
	r := &controller.PocketIDInstanceReconciler{
		Client: testClient,
		Scheme: testClient.Scheme(),
	}
	return r.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: namespace},
	})
}

func getUser(name, namespace string) *pocketidv1alpha1.PocketIDUser {
	user := &pocketidv1alpha1.PocketIDUser{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, user)
	Expect(err).NotTo(HaveOccurred())
	return user
}

func getFreshUser(name, namespace string) *pocketidv1alpha1.PocketIDUser {
	user := &pocketidv1alpha1.PocketIDUser{}
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, user)
	return user
}

func getUserGroup(name, namespace string) *pocketidv1alpha1.PocketIDUserGroup {
	group := &pocketidv1alpha1.PocketIDUserGroup{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, group)
	Expect(err).NotTo(HaveOccurred())
	return group
}

func getFreshUserGroup(name, namespace string) *pocketidv1alpha1.PocketIDUserGroup {
	group := &pocketidv1alpha1.PocketIDUserGroup{}
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, group)
	return group
}

func getOIDCClient(name, namespace string) *pocketidv1alpha1.PocketIDOIDCClient {
	client := &pocketidv1alpha1.PocketIDOIDCClient{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, client)
	Expect(err).NotTo(HaveOccurred())
	return client
}

func getFreshOIDCClient(name, namespace string) *pocketidv1alpha1.PocketIDOIDCClient {
	client := &pocketidv1alpha1.PocketIDOIDCClient{}
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, client)
	return client
}

func getInstance(name, namespace string) *pocketidv1alpha1.PocketIDInstance {
	instance := &pocketidv1alpha1.PocketIDInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, instance)
	Expect(err).NotTo(HaveOccurred())
	return instance
}

func getFreshInstance(name, namespace string) *pocketidv1alpha1.PocketIDInstance {
	instance := &pocketidv1alpha1.PocketIDInstance{}
	_ = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, instance)
	return instance
}

func cleanupUser(name, namespace string) {
	user := &pocketidv1alpha1.PocketIDUser{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, user)
	if err == nil {
		k8sClient.Delete(ctx, user)
	}
}

func cleanupUserGroup(name, namespace string) {
	group := &pocketidv1alpha1.PocketIDUserGroup{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, group)
	if err == nil {
		k8sClient.Delete(ctx, group)
	}
}

func cleanupOIDCClient(name, namespace string) {
	client := &pocketidv1alpha1.PocketIDOIDCClient{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, client)
	if err == nil {
		k8sClient.Delete(ctx, client)
	}
}

func cleanupInstance(name, namespace string) {
	instance := &pocketidv1alpha1.PocketIDInstance{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, instance)
	if err == nil {
		k8sClient.Delete(ctx, instance)
	}
}

var _ = Describe("PocketIDUser Reconciliation", func() {
	var (
		userName     string
		namespace    string
		instanceName string
		secretName   string
	)

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-user-%d", time.Now().UnixNano())
		userName = "test-user"
		instanceName = "test-instance"
		secretName = instanceName + "-api-key"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		createAPIKeySecret(secretName, namespace, "test-api-key")
		createTestInstance(instanceName, namespace)
	})

	AfterEach(func() {
		cleanupUser(userName, namespace)
		cleanupInstance(instanceName, namespace)
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	})

	It("should create user in PocketID when CR is created", func() {
		email := "test@example.com"
		user := &pocketidv1alpha1.PocketIDUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDUserSpec{
				Username: userName,
				Email:    &email,
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
			},
		}
		err := k8sClient.Create(ctx, user)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileUser(userName, namespace)
		Expect(err).NotTo(HaveOccurred())

		fetched := getUser(userName, namespace)
		Expect(fetched.Status.UserID).NotTo(BeEmpty())

		calls := mockServer.CallLog()
		Expect(calls).To(ContainElement(ContainSubstring("POST /api/users")))
	})

	It("should set Ready condition when user is synced", func() {
		email := "test@example.com"
		user := &pocketidv1alpha1.PocketIDUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDUserSpec{
				Username: userName,
				Email:    &email,
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
			},
		}
		err := k8sClient.Create(ctx, user)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileUser(userName, namespace)
		Expect(err).NotTo(HaveOccurred())

		fetched := getFreshUser(userName, namespace)
		Expect(fetched.Status.Ready).To(BeTrue())
	})

	It("should handle API errors and return error", func() {
		mockServer.SetError("CreateUser", fmt.Errorf("API error: status=500 message=\"internal server error\""))

		email := "test@example.com"
		user := &pocketidv1alpha1.PocketIDUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDUserSpec{
				Username: userName,
				Email:    &email,
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
			},
		}
		err := k8sClient.Create(ctx, user)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileUser(userName, namespace)
		Expect(err).To(HaveOccurred())
	})

	It("should delete user from PocketID when CR is deleted", func() {
		email := "test@example.com"
		user := &pocketidv1alpha1.PocketIDUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDUserSpec{
				Username: userName,
				Email:    &email,
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
			},
		}
		err := k8sClient.Create(ctx, user)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileUser(userName, namespace)
		Expect(err).NotTo(HaveOccurred())

		fetched := getUser(userName, namespace)
		userID := fetched.Status.UserID
		Expect(userID).NotTo(BeEmpty())

		now := metav1.Now()
		fetched.DeletionTimestamp = &now
		fetched.Finalizers = []string{"pocketid.tobiash.github.io/user-finalizer"}
		err = k8sClient.Update(ctx, fetched)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileUser(userName, namespace)
		Expect(err).NotTo(HaveOccurred())

		calls := mockServer.CallLog()
		Expect(calls).To(ContainElement(ContainSubstring("DELETE /api/users/")))
	})
})

var _ = Describe("PocketIDOIDCClient Reconciliation", func() {
	var (
		clientName   string
		namespace    string
		instanceName string
		secretName   string
	)

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-oidc-%d", time.Now().UnixNano())
		clientName = "test-client"
		instanceName = namespace // Use namespace as part of instance name to ensure uniqueness
		secretName = instanceName + "-api-key"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		createAPIKeySecret(secretName, namespace, "test-api-key")
		createTestInstance(instanceName, namespace)
	})

	AfterEach(func() {
		cleanupOIDCClient(clientName, namespace)
		cleanupInstance(instanceName, namespace)
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	})

	It("should create OIDC client and secret", func() {
		secretRef := &pocketidv1alpha1.LocalObjectReference{Name: "test-credentials"}
		oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
				Name: "Test Application",
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
				CallbackURLs:         []string{"https://example.com/callback"},
				CredentialsSecretRef: secretRef,
			},
		}
		err := k8sClient.Create(ctx, oidcClient)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileOIDCClient(clientName, namespace)
		Expect(err).NotTo(HaveOccurred())

		fetched := getFreshOIDCClient(clientName, namespace)
		Expect(fetched.Status.ClientID).NotTo(BeEmpty())
		Expect(fetched.Status.CredentialsSecretName).To(Equal("test-credentials"))

		calls := mockServer.CallLog()
		Expect(calls).To(ContainElement("POST /api/oidc/clients"))
		Expect(calls).To(ContainElement(ContainSubstring("/secret")))
	})

	It("should set Ready condition when client is synced", func() {
		secretRef := &pocketidv1alpha1.LocalObjectReference{Name: "test-credentials"}
		oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
				Name: "Test Application",
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
				CallbackURLs:         []string{"https://example.com/callback"},
				CredentialsSecretRef: secretRef,
			},
		}
		err := k8sClient.Create(ctx, oidcClient)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileOIDCClient(clientName, namespace)
		Expect(err).NotTo(HaveOccurred())

		fetched := getFreshOIDCClient(clientName, namespace)
		Expect(fetched.Status.Ready).To(BeTrue())
	})

	It("should handle API errors", func() {
		mockServer.SetError("CreateOIDCClient", fmt.Errorf("API error: status=500 message=\"internal error\""))

		secretRef := &pocketidv1alpha1.LocalObjectReference{Name: "test-credentials"}
		oidcClient := &pocketidv1alpha1.PocketIDOIDCClient{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDOIDCClientSpec{
				Name: "Test Application",
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
				CallbackURLs:         []string{"https://example.com/callback"},
				CredentialsSecretRef: secretRef,
			},
		}
		err := k8sClient.Create(ctx, oidcClient)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileOIDCClient(clientName, namespace)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("PocketIDUserGroup Reconciliation", func() {
	var (
		groupName    string
		namespace    string
		instanceName string
		secretName   string
	)

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-group-%d", time.Now().UnixNano())
		groupName = "test-group"
		instanceName = "test-instance"
		secretName = instanceName + "-api-key"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())

		createAPIKeySecret(secretName, namespace, "test-api-key")
		createTestInstance(instanceName, namespace)
	})

	AfterEach(func() {
		cleanupUserGroup(groupName, namespace)
		cleanupInstance(instanceName, namespace)
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	})

	It("should create group in PocketID when CR is created", func() {
		group := &pocketidv1alpha1.PocketIDUserGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      groupName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDUserGroupSpec{
				Name: groupName,
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
			},
		}
		err := k8sClient.Create(ctx, group)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileUserGroup(groupName, namespace)
		Expect(err).NotTo(HaveOccurred())

		fetched := getFreshUserGroup(groupName, namespace)
		Expect(fetched.Status.GroupID).NotTo(BeEmpty())

		calls := mockServer.CallLog()
		Expect(calls).To(ContainElement(ContainSubstring("POST /api/groups")))
	})

	It("should handle API errors", func() {
		mockServer.SetError("CreateGroup", fmt.Errorf("API error: status=500 message=\"internal error\""))

		group := &pocketidv1alpha1.PocketIDUserGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      groupName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDUserGroupSpec{
				Name: groupName,
				InstanceRef: pocketidv1alpha1.CrossNamespaceObjectReference{
					Name: instanceName,
				},
			},
		}
		err := k8sClient.Create(ctx, group)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileUserGroup(groupName, namespace)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("PocketIDInstance Reconciliation", func() {
	var (
		instanceName string
		namespace    string
	)

	BeforeEach(func() {
		namespace = fmt.Sprintf("test-instance-%d", time.Now().UnixNano())
		instanceName = "test-instance"

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		err := k8sClient.Create(ctx, ns)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		cleanupInstance(instanceName, namespace)
		_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	})

	It("should create API key secret when instance is created", func() {
		instance := &pocketidv1alpha1.PocketIDInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDInstanceSpec{
				AppURL: "https://auth.example.com",
			},
		}
		err := k8sClient.Create(ctx, instance)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileInstance(instanceName, namespace)
		Expect(err).NotTo(HaveOccurred())

		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: instanceName + "-api-key", Namespace: namespace}, secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(secret.Data["STATIC_API_KEY"]).NotTo(BeEmpty())
	})

	It("should set Ready condition when instance is healthy", func() {
		instance := &pocketidv1alpha1.PocketIDInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: namespace,
			},
			Spec: pocketidv1alpha1.PocketIDInstanceSpec{
				AppURL: "https://auth.example.com",
			},
		}
		err := k8sClient.Create(ctx, instance)
		Expect(err).NotTo(HaveOccurred())

		_, err = reconcileInstance(instanceName, namespace)
		Expect(err).NotTo(HaveOccurred())

		fetched := getFreshInstance(instanceName, namespace)
		Expect(fetched.Status.Ready).To(BeTrue())
	})
})
