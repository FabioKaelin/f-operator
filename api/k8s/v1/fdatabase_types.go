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

// FdatabaseSpec defines the desired state of Fdatabase
type FdatabaseSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Database     DynamicConfig `json:"database"`
	RootHost     string        `json:"rootHost,omitempty"`
	User         DynamicConfig `json:"user"`
	RootPassword DynamicConfig `json:"rootPassword"`
	Password     DynamicConfig `json:"password"`
}

type DynamicConfig struct {
	Value      string        `json:"value,omitempty"`
	FromConfig FromReference `json:"fromConfig,omitempty"`
	FromSecret FromReference `json:"fromSecret,omitempty"`
}

// FdatabaseStatus defines the observed state of Fdatabase
type FdatabaseStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// Fdatabase is the Schema for the fdatabases API

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Database",type="string",JSONPath=".spec.database",description="What is the name of the database"
// +kubebuilder:printcolumn:name="User",type="string",JSONPath=".spec.user",description="Which user should be used"
type Fdatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FdatabaseSpec   `json:"spec,omitempty"`
	Status FdatabaseStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// FdatabaseList contains a list of Fdatabase
type FdatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Fdatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Fdatabase{}, &FdatabaseList{})
}
