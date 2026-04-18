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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PocketIDOIDCClientSpec defines the desired state of PocketIDOIDCClient
type PocketIDOIDCClientSpec struct {
	// InstanceRef references the PocketIDInstance this client belongs to
	// Can reference instances in other namespaces if allowed by the instance
	// +kubebuilder:validation:Required
	InstanceRef CrossNamespaceObjectReference `json:"instanceRef"`

	// Name is a human-readable name for the OIDC client
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ClientID is the OIDC client identifier (optional, defaults to metadata.name)
	ClientID string `json:"clientId,omitempty"`

	// IsPublic indicates if this is a public client (no secret required)
	// +kubebuilder:default=false
	IsPublic bool `json:"isPublic,omitempty"`

	// CallbackURLs are the allowed redirect URIs after authentication
	// +kubebuilder:validation:MinItems=1
	CallbackURLs []string `json:"callbackURLs"`

	// LogoutCallbackURLs are the allowed redirect URIs after logout
	LogoutCallbackURLs []string `json:"logoutCallbackURLs,omitempty"`

	// AllowedUserGroupRefs restricts access to specified groups
	AllowedUserGroupRefs []LocalObjectReference `json:"allowedUserGroupRefs,omitempty"`

	// CredentialsSecretRef specifies where to store the client credentials
	// The operator will create/update this Secret with client_id, client_secret, and issuer_url
	CredentialsSecretRef *LocalObjectReference `json:"credentialsSecretRef,omitempty"`

	// PKCEEnabled requires Proof Key for Code Exchange
	// +kubebuilder:default=true
	PKCEEnabled bool `json:"pkceEnabled,omitempty"`

	// HTTPRouteSelector watches HTTPRoutes with matching labels for auto-configuration
	HTTPRouteSelector *metav1.LabelSelector `json:"httpRouteSelector,omitempty"`

	// EnvoyGateway configuration for automatic SecurityPolicy creation
	EnvoyGateway *EnvoyGatewayConfig `json:"envoyGateway,omitempty"`
}

// EnvoyGatewayConfig configures automatic Envoy Gateway SecurityPolicy creation
type EnvoyGatewayConfig struct {
	// Enabled enables automatic SecurityPolicy creation
	Enabled bool `json:"enabled"`

	// HTTPRouteRef references the HTTPRoute to protect with OIDC
	HTTPRouteRef *NamespacedObjectReference `json:"httpRouteRef,omitempty"`

	// CallbackPath is the OAuth callback path (default: /oauth2/callback)
	// +kubebuilder:default="/oauth2/callback"
	CallbackPath string `json:"callbackPath,omitempty"`

	// LogoutPath is the logout path (default: /logout)
	// +kubebuilder:default="/logout"
	LogoutPath string `json:"logoutPath,omitempty"`
}

// NamespacedObjectReference references an object in a specific namespace
type NamespacedObjectReference struct {
	// Name of the referenced object
	Name string `json:"name"`
	// Namespace of the referenced object (optional, defaults to same namespace)
	Namespace string `json:"namespace,omitempty"`
}

// PocketIDOIDCClientStatus defines the observed state of PocketIDOIDCClient
type PocketIDOIDCClientStatus struct {
	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates if the client is synced with PocketID
	Ready bool `json:"ready"`

	// ClientID is the OIDC client identifier
	ClientID string `json:"clientId,omitempty"`

	// Synced indicates the client is successfully synchronized
	Synced bool `json:"synced"`

	// LastSyncTime is when the client was last synchronized
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// CredentialsSecretName is the name of the Secret containing credentials
	CredentialsSecretName string `json:"credentialsSecretName,omitempty"`

	// SecurityPolicyName is the name of the created Envoy Gateway SecurityPolicy
	SecurityPolicyName string `json:"securityPolicyName,omitempty"`

	// ObservedGeneration is the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Client Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Public",type="boolean",JSONPath=".spec.isPublic"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PocketIDOIDCClient is the Schema for the pocketidoidcclients API
type PocketIDOIDCClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PocketIDOIDCClientSpec   `json:"spec,omitempty"`
	Status PocketIDOIDCClientStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PocketIDOIDCClientList contains a list of PocketIDOIDCClient
type PocketIDOIDCClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PocketIDOIDCClient `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PocketIDOIDCClient{}, &PocketIDOIDCClientList{})
}
