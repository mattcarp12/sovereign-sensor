package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=allow;log;block-kill;block-noconn
type Action string

const (
	ActionAllow       Action = "allow"
	ActionLog         Action = "log"
	ActionBlock       Action = "block-kill"
	ActionBlockNoConn Action = "block-noconn"
)

// SovereigntyPolicySpec defines the desired state of SovereigntyPolicy
type SovereigntyPolicySpec struct {
	// Actions defines what to do when a policy matches.
	// Valid values: "log", "block-kill" (SIGKILL process), "block-noconn" (close connection)
	Actions []Action `json:"actions"`

	// Namespaces defines which Kubernetes namespaces this policy applies to
	Namespaces []string `json:"namespaces,omitempty"`

	// Description provides human-readable context for the policy
	Description string `json:"description,omitempty"`

	// AllowedCountries defines the ISO-3166-1 alpha-2 country codes that are permitted
	AllowedCountries []string `json:"allowedCountries,omitempty"`

	// DisallowedCountries defines the ISO-3166-1 alpha-2 country codes that are explicitly blocked
	DisallowedCountries []string `json:"disallowedCountries,omitempty"`
}

// SovereigntyPolicyStatus defines the observed state of SovereigntyPolicy
type SovereigntyPolicyStatus struct {
	// Conditions represent the latest available observations of an object's state
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// DiscoveredViolatorIPs is populated by the Data Plane Agent.
	// The Reconciler uses this list to dynamically generate Tetragon TracingPolicies.
	// +listType=set
	DiscoveredViolatorIPs []string `json:"discoveredViolatorIPs,omitempty"`
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
