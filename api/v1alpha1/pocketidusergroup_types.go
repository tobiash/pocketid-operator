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

// PocketIDUserGroupSpec defines the desired state of PocketIDUserGroup
type PocketIDUserGroupSpec struct {
	// InstanceRef references the PocketIDInstance this group belongs to
	// Can reference instances in other namespaces if allowed by the instance
	// +kubebuilder:validation:Required
	InstanceRef CrossNamespaceObjectReference `json:"instanceRef"`

	// Name is the display name of the group
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// FriendlyName is an optional human-friendly name
	FriendlyName string `json:"friendlyName,omitempty"`

	// IsDefault makes this group automatically assigned to new users
	// +kubebuilder:default=false
	IsDefault bool `json:"isDefault,omitempty"`

	// CustomClaims to add to tokens for users in this group
	CustomClaims []CustomClaim `json:"customClaims,omitempty"`
}

// CustomClaim defines a custom claim to add to tokens
type CustomClaim struct {
	// Key is the claim key
	Key string `json:"key"`
	// Value is the claim value
	Value string `json:"value"`
}

// PocketIDUserGroupStatus defines the observed state of PocketIDUserGroup
type PocketIDUserGroupStatus struct {
	// Conditions represent the latest available observations
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Ready indicates if the group is synced with PocketID
	Ready bool `json:"ready"`

	// GroupID is the PocketID internal group ID
	GroupID string `json:"groupId,omitempty"`

	// UserCount is the number of users in this group
	UserCount int `json:"userCount"`

	// Synced indicates the group is successfully synchronized
	Synced bool `json:"synced"`

	// LastSyncTime is when the group was last synchronized
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration is the most recent generation observed
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Name",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Default",type="boolean",JSONPath=".spec.isDefault"
// +kubebuilder:printcolumn:name="Users",type="integer",JSONPath=".status.userCount"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PocketIDUserGroup is the Schema for the pocketidusergroups API
type PocketIDUserGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PocketIDUserGroupSpec   `json:"spec,omitempty"`
	Status PocketIDUserGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PocketIDUserGroupList contains a list of PocketIDUserGroup
type PocketIDUserGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PocketIDUserGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PocketIDUserGroup{}, &PocketIDUserGroupList{})
}
