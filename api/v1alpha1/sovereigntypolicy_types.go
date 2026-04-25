package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=allow;log;block
type Action string

const (
	ActionAllow Action = "allow"
	ActionLog   Action = "log"
	ActionBlock Action = "block"
)

// SovereigntyPolicySpec defines the desired state of SovereigntyPolicy
type SovereigntyPolicySpec struct {
	// +kubebuilder:validation:Required
	Action Action `json:"action"`

	// +kubebuilder:validation:MinItems=1
	Namespaces []string `json:"namespaces"`

	// +optional
	AllowedCountries []string `json:"allowedCountries,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`
}

// SovereigntyPolicyStatus defines the observed state of SovereigntyPolicy
type SovereigntyPolicyStatus struct {
	// Represents the current state of the policy across the cluster (e.g., "Active", "Error")
	// +optional
	State string `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=sp

// SovereigntyPolicy is the Schema for the sovereigntypolicies API
type SovereigntyPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SovereigntyPolicySpec   `json:"spec,omitempty"`
	Status SovereigntyPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SovereigntyPolicyList contains a list of SovereigntyPolicy
type SovereigntyPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SovereigntyPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SovereigntyPolicy{}, &SovereigntyPolicyList{})
}
