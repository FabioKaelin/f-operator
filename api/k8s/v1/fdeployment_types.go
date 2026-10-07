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

	// +kubebuilder:validation:Pattern=`^/`
	Path string `json:"path"`

	Host string `json:"host"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:validation:ExclusiveMaximum=false

	Replicas int32 `json:"replicas"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	Image string `json:"image,omitempty"`
	Tag   string `json:"tag,omitempty"`

	Security FdeploymentSecurity `json:"security,omitempty"`

	Resources FdeploymentResources `json:"resources"`

	HealthCheck FdeploymentHealthCheck `json:"healthCheck"`

	Environments []Environment `json:"env,omitempty"`
}

// FdeploymentSecurity allows explicit compatibility exceptions for application images.
// Root images remain supported; blanket privilege is disabled by default.
// +kubebuilder:validation:XValidation:rule="!has(self.nginxCompatibility) || !self.nginxCompatibility || !has(self.runAsNonRoot) || !self.runAsNonRoot",message="nginxCompatibility requires the standard root master process"
type FdeploymentSecurity struct {
	// NginxCompatibility permits a root master to initialize files and switch workers
	// to the nginx user without granting privileged container access.
	NginxCompatibility bool `json:"nginxCompatibility,omitempty"`
	Privileged         bool `json:"privileged,omitempty"`
	RunAsNonRoot       bool `json:"runAsNonRoot,omitempty"`
}

type FdeploymentHealthCheck struct {
	LivenessProbe  HealthProbe `json:"livenessProbe"`
	ReadinessProbe HealthProbe `json:"readinessProbe"`
}

type HealthProbe struct {
	Path string `json:"path"`
}

// +kubebuilder:validation:XValidation:rule="quantity(self.requests.cpu).compareTo(quantity(self.limits.cpu)) <= 0",message="CPU request must not exceed limit"
// +kubebuilder:validation:XValidation:rule="quantity(self.requests.memory).compareTo(quantity(self.limits.memory)) <= 0",message="memory request must not exceed limit"
type FdeploymentResources struct {
	Requests Resource `json:"requests"`

	Limits Resource `json:"limits"`
}

// +kubebuilder:validation:XValidation:rule="isQuantity(self.cpu) && quantity(self.cpu).isGreaterThan(quantity('0'))",message="cpu must be a positive Kubernetes quantity"
// +kubebuilder:validation:XValidation:rule="isQuantity(self.memory) && quantity(self.memory).isGreaterThan(quantity('0'))",message="memory must be a positive Kubernetes quantity"
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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FdeploymentSpec   `json:"spec,omitempty"`
	Status FdeploymentStatus `json:"status,omitempty"`
}

// // DeepCopyObject implements client.Object.
// func (*Fdeployment) DeepCopyObject() runtime.Object {
// 	panic("unimplemented")
// }

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
