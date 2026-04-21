package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:validation:Enum=allow;log;block
type Action string

const (
	ActionAllow Action = "allow"
	ActionLog   Action = "log"
	ActionBlock Action = "block"
)

// PolicySpec defines the desired state of the SovereigntyPolicy.
type PolicySpec struct {
	// Action defines the enforcement behavior when a violation occurs.
	// +kubebuilder:validation:Required
	Action Action `json:"action"`

	// Namespaces to which this policy applies. Use "*" for a global default.
	// +kubebuilder:validation:MinItems=1
	Namespaces []string `json:"namespaces"`

	// AllowedCountries is a list of ISO 3166-1 alpha-2 country codes.
	// +optional
	AllowedCountries []string `json:"allowedCountries,omitempty"`

	// Description provides human-readable context for the rule.
	// +optional
	Description string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=sp

// SovereigntyPolicy is the Schema for the sovereigntypolicies API.
type SovereigntyPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// SovereigntyPolicyList contains a list of SovereigntyPolicy objects.
type SovereigntyPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SovereigntyPolicy `json:"items"`
}
