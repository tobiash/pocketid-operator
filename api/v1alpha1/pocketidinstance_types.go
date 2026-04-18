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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PocketIDInstanceSpec defines the desired state of PocketIDInstance
type PocketIDInstanceSpec struct {
	// AppURL is the public URL where PocketID will be accessible (e.g., https://auth.example.com)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	AppURL string `json:"appUrl"`

	// Image specifies the PocketID container image to use
	// +kubebuilder:default="ghcr.io/pocket-id/pocket-id:latest"
	Image string `json:"image,omitempty"`

	// Replicas is the number of PocketID instances to run
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Replicas *int32 `json:"replicas,omitempty"`

	// EncryptionKeySecretRef references a Secret containing the ENCRYPTION_KEY
	// If not set, operator will auto-generate one
	EncryptionKeySecretRef *SecretKeySelector `json:"encryptionKeySecretRef,omitempty"`

	// StaticAPIKeySecretRef references a Secret containing the STATIC_API_KEY
	// Used by the operator to manage PocketID resources via API
	// If not set, operator will auto-generate one
	StaticAPIKeySecretRef *SecretKeySelector `json:"staticApiKeySecretRef,omitempty"`

	// Database configuration
	Database DatabaseConfig `json:"database,omitempty"`

	// Storage configuration for data persistence
	Storage StorageConfig `json:"storage,omitempty"`

	// SMTP configuration for email sending
	SMTP *SMTPConfig `json:"smtp,omitempty"`

	// SessionDuration in minutes (default: 60)
	// +kubebuilder:default=60
	SessionDuration int `json:"sessionDuration,omitempty"`

	// TrustProxy enables trusting X-Forwarded-* headers
	// +kubebuilder:default=true
	TrustProxy bool `json:"trustProxy,omitempty"`

	// Resources defines the resource requirements for the PocketID container
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// InitialAdmin configuration for the first user
	// Only used during initial setup
	InitialAdmin *InitialAdminConfig `json:"initialAdmin,omitempty"`

	// AllowedReferences defines which namespaces can reference this instance
	// Similar to Gateway API's allowedRoutes pattern
	// +optional
	AllowedReferences *AllowedReferences `json:"allowedReferences,omitempty"`
}

// InitialAdminConfig defines the initial administrator user
type InitialAdminConfig struct {
	// +kubebuilder:validation:Required
	Username string `json:"username"`
	// +kubebuilder:validation:Required
	Email string `json:"email"`
	// +kubebuilder:validation:Required
	FirstName string `json:"firstName"`
	// +kubebuilder:validation:Required
	DisplayName string `json:"displayName"`
}

// SecretKeySelector references a key in a Secret
type SecretKeySelector struct {
	// Name of the Secret
	Name string `json:"name"`
	// Key within the Secret
	Key string `json:"key"`
}

// DatabaseConfig configures the database connection
type DatabaseConfig struct {
	// Provider is the database type: sqlite or postgres
	// +kubebuilder:default="sqlite"
	// +kubebuilder:validation:Enum=sqlite;postgres
	Provider string `json:"provider,omitempty"`

	// PostgresSecretRef references a Secret containing PostgreSQL connection details
	// Required when provider is "postgres"
	PostgresSecretRef *LocalObjectReference `json:"postgresSecretRef,omitempty"`
}

// StorageConfig configures data storage
type StorageConfig struct {
	// PersistentVolumeClaim configuration for local storage
	PVC *PVCConfig `json:"pvc,omitempty"`

	// S3 configuration for object storage
	S3 *S3Config `json:"s3,omitempty"`
}

// PVCConfig configures PersistentVolumeClaim storage
type PVCConfig struct {
	// StorageClassName for the PVC
	StorageClassName *string `json:"storageClassName,omitempty"`
	// Size of the PVC (e.g., "1Gi")
	// +kubebuilder:default="1Gi"
	Size string `json:"size,omitempty"`
}

// S3Config configures S3-compatible object storage
type S3Config struct {
	// Endpoint for S3-compatible storage
	Endpoint string `json:"endpoint"`
	// Bucket name
	Bucket string `json:"bucket"`
	// Region
	Region string `json:"region,omitempty"`
	// SecretRef contains access key and secret key
	SecretRef LocalObjectReference `json:"secretRef"`
}

// SMTPConfig configures email sending
type SMTPConfig struct {
	// Host is the SMTP server hostname
	Host string `json:"host"`
	// Port is the SMTP server port
	// +kubebuilder:default=587
	Port int `json:"port,omitempty"`
	// From is the sender email address
	From string `json:"from"`
	// SecretRef contains SMTP username and password
	SecretRef *LocalObjectReference `json:"secretRef,omitempty"`
	// TLS enables STARTTLS
	// +kubebuilder:default=true
	TLS bool `json:"tls,omitempty"`
}

// LocalObjectReference references an object in the same namespace
type LocalObjectReference struct {
	// Name of the referenced object
	Name string `json:"name"`
}

// AllowedReferences defines cross-namespace reference permissions
type AllowedReferences struct {
	// Namespaces indicates which namespaces are allowed to reference this instance
	// +optional
	Namespaces *NamespacesFrom `json:"namespaces,omitempty"`
}

// NamespacesFrom indicates namespace selection strategy
type NamespacesFrom struct {
	// From indicates where namespaces are selected
	// +optional
	// +kubebuilder:default=Same
	From *FromNamespaces `json:"from,omitempty"`

	// Selector matches namespaces by label
	// Only valid when From=Selector
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// FromNamespaces defines namespace selection strategies
// +kubebuilder:validation:Enum=All;Same;Selector
type FromNamespaces string

const (
	// NamespacesFromAll allows references from all namespaces
	NamespacesFromAll FromNamespaces = "All"

	// NamespacesFromSame allows references only from the same namespace (default)
	NamespacesFromSame FromNamespaces = "Same"

	// NamespacesFromSelector allows references from namespaces matching selector
	NamespacesFromSelector FromNamespaces = "Selector"
)

// CrossNamespaceObjectReference references an object that may be in another namespace
type CrossNamespaceObjectReference struct {
	// Name of the referenced object
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the referenced object
	// When unspecified, refers to the local namespace
	// +optional
	Namespace *string `json:"namespace,omitempty"`
}

// PocketIDInstanceStatus defines the observed state of PocketIDInstance
type PocketIDInstanceStatus struct {
	// Conditions represent the latest available observations of the instance's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates if the instance is ready to accept requests
	Ready bool `json:"ready"`

	// InternalURL is the cluster-internal URL for API access
	InternalURL string `json:"internalUrl,omitempty"`

	// Version is the detected PocketID version
	Version string `json:"version,omitempty"`

	// StaticAPIKeySecretName is the name of the Secret containing the static API key
	StaticAPIKeySecretName string `json:"staticApiKeySecretName,omitempty"`

	// ObservedGeneration is the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// Condition types for PocketIDInstance
const (
	// ConditionTypeAvailable indicates the instance is available
	ConditionTypeAvailable = "Available"
	// ConditionTypeConfigured indicates configuration is valid
	ConditionTypeConfigured = "Configured"
	// ConditionTypeDegraded indicates the instance is in a degraded state
	ConditionTypeDegraded = "Degraded"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.appUrl"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PocketIDInstance is the Schema for the pocketidinstances API
type PocketIDInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PocketIDInstanceSpec   `json:"spec,omitempty"`
	Status PocketIDInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PocketIDInstanceList contains a list of PocketIDInstance
type PocketIDInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PocketIDInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PocketIDInstance{}, &PocketIDInstanceList{})
}
