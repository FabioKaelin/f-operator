/*
Copyright 2023.

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// FdeploymentSpec defines the desired state of Fdeployment
type FdeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Path string `json:"path"`

	Host string `json:"host"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:validation:ExclusiveMaximum=false

	Replicas int32 `json:"replicas"`

	Port int32 `json:"port"`

	Tag string `json:"tag,omitempty"`

	Resources FdeploymentResources `json:"resources"`

	HealthCheck FdeploymentHealthCheck `json:"healthCheck"`

	Environments []Environment `json:"env"`
}

type FdeploymentHealthCheck struct {
	LivenessProbe  HealthProbe `json:"livenessProbe"`
	ReadinessProbe HealthProbe `json:"readinessProbe"`
}

type HealthProbe struct {
	Path string `json:"path"`
}

type FdeploymentResources struct {
	Requests Resource `json:"requests"`

	Limits Resource `json:"limits"`
}

type Resource struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

type Environment struct {
	Name       string        `json:"name"`
	Value      string        `json:"value,omitempty"`
	FromConfig FromReference `json:"fromConfig,omitempty"`
	FromSecret FromReference `json:"fromSecret,omitempty"`
}

type FromReference struct {
	Name string `json:"name"` // name of the configmap
	Key  string `json:"key"`  // key of the configmap
}

// FdeploymentStatus defines the observed state of Fdeployment
type FdeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Represents the observations of a Fdeployment's current state.
	// Fdeployment.status.conditions.type are: "Available", "Progressing", and "Degraded"
	// Fdeployment.status.conditions.status are one of True, False, Unknown.
	// Fdeployment.status.conditions.reason the value should be a CamelCase string and producers of specific
	// condition types may define expected values and meanings for this field, and whether the values
	// are considered a guaranteed API.
	// Fdeployment.status.conditions.Message is a human readable message indicating details about the transition.
	// For further information see: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// Fdeployment is the Schema for the fdeployments API

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas",description="How many replicas has this deployment"
// +kubebuilder:printcolumn:name="Host",type="string",JSONPath=".spec.host",description="Which host has this deployment"
// +kubebuilder:printcolumn:name="Path",type="string",JSONPath=".spec.path",description="Which subpath has this deployment"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Tag",type="string",JSONPath=".spec.tag",description="Which image tag is deployed",priority=1
// +kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.port",description="Which port is targeted",priority=1
type Fdeployment struct {
	// TODO: Add ready to print columns (most likely from status (which also has to be done first))
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FdeploymentSpec   `json:"spec,omitempty"`
	Status FdeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FdeploymentList contains a list of Fdeployment
type FdeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Fdeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Fdeployment{}, &FdeploymentList{})
}
