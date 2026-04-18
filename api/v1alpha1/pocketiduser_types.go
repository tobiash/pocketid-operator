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

// PocketIDUserSpec defines the desired state of PocketIDUser
type PocketIDUserSpec struct {
	// InstanceRef references the PocketIDInstance this user belongs to
	// Can reference instances in other namespaces if allowed by the instance
	// +kubebuilder:validation:Required
	InstanceRef CrossNamespaceObjectReference `json:"instanceRef"`

	// Username is the unique username for the user
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Username string `json:"username"`

	// Email is the user's email address
	Email *string `json:"email,omitempty"`

	// FirstName of the user
	FirstName string `json:"firstName,omitempty"`

	// LastName of the user
	LastName string `json:"lastName,omitempty"`

	// DisplayName for the user (optional, computed from first/last if not set)
	DisplayName string `json:"displayName,omitempty"`

	// IsAdmin grants administrative privileges
	// +kubebuilder:default=false
	IsAdmin bool `json:"isAdmin,omitempty"`

	// Disabled prevents the user from logging in
	// +kubebuilder:default=false
	Disabled bool `json:"disabled,omitempty"`

	// Locale for the user (e.g., "en", "de")
	Locale string `json:"locale,omitempty"`

	// UserGroupRefs references PocketIDUserGroup resources for group membership
	UserGroupRefs []LocalObjectReference `json:"userGroupRefs,omitempty"`

	// SendOnboardingEmail triggers a one-time access email after creation
	// Requires SMTP to be configured on the PocketIDInstance
	SendOnboardingEmail bool `json:"sendOnboardingEmail,omitempty"`

	// OneTimeAccessSecretRef - if set, operator writes the one-time access link
	// to this secret instead of/in addition to sending email.
	// Useful for testing without SMTP or programmatic onboarding.
	OneTimeAccessSecretRef *LocalObjectReference `json:"oneTimeAccessSecretRef,omitempty"`
}

// PocketIDUserStatus defines the observed state of PocketIDUser
type PocketIDUserStatus struct {
	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates if the user is synced with PocketID
	Ready bool `json:"ready"`

	// UserID is the PocketID internal user ID
	UserID string `json:"userId,omitempty"`

	// Synced indicates the user is successfully synchronized
	Synced bool `json:"synced"`

	// LastSyncTime is when the user was last synchronized
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// OnboardingEmailSent indicates if the onboarding email was sent
	OnboardingEmailSent bool `json:"onboardingEmailSent,omitempty"`

	// OnboardingEmailSentAt is when the email was sent
	OnboardingEmailSentAt *metav1.Time `json:"onboardingEmailSentAt,omitempty"`

	// ObservedGeneration is the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Username",type="string",JSONPath=".spec.username"
// +kubebuilder:printcolumn:name="Email",type="string",JSONPath=".spec.email"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PocketIDUser is the Schema for the pocketidusers API
type PocketIDUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PocketIDUserSpec   `json:"spec,omitempty"`
	Status PocketIDUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PocketIDUserList contains a list of PocketIDUser
type PocketIDUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PocketIDUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PocketIDUser{}, &PocketIDUserList{})
}
