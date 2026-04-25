package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SovereignSensorSpec defines the desired state of SovereignSensor
type SovereignSensorSpec struct {
	// Instructs the Operator to automatically deploy the Tetragon DaemonSet alongside the sensor.
	// +kubebuilder:default=true
	DeployTetragon bool `json:"deployTetragon"`

	// Controls the verbosity of the sensor agents.
	// +kubebuilder:default="INFO"
	LogLevel string `json:"logLevel,omitempty"`

	// The namespace where the DaemonSets should be deployed.
	// +kubebuilder:default="kube-system"
	TargetNamespace string `json:"targetNamespace,omitempty"`
}

// SovereignSensorStatus defines the observed state of SovereignSensor
type SovereignSensorStatus struct {
	// Tracks how many nodes currently have a healthy sensor agent running.
	// +optional
	ActiveAgents int32 `json:"activeAgents,omitempty"`

	// Reports the overarching health of the sensor deployment.
	// +optional
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ss

// SovereignSensor is the Schema for the sovereignsensors API
type SovereignSensor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SovereignSensorSpec   `json:"spec,omitempty"`
	Status SovereignSensorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SovereignSensorList contains a list of SovereignSensor
type SovereignSensorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SovereignSensor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SovereignSensor{}, &SovereignSensorList{})
}
