//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DynamicConfig) DeepCopyInto(out *DynamicConfig) {
	*out = *in
	out.FromConfig = in.FromConfig
	out.FromSecret = in.FromSecret
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DynamicConfig.
func (in *DynamicConfig) DeepCopy() *DynamicConfig {
	if in == nil {
		return nil
	}
	out := new(DynamicConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Environment) DeepCopyInto(out *Environment) {
	*out = *in
	out.FromConfig = in.FromConfig
	out.FromSecret = in.FromSecret
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Environment.
func (in *Environment) DeepCopy() *Environment {
	if in == nil {
		return nil
	}
	out := new(Environment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Fdatabase) DeepCopyInto(out *Fdatabase) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Fdatabase.
func (in *Fdatabase) DeepCopy() *Fdatabase {
	if in == nil {
		return nil
	}
	out := new(Fdatabase)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Fdatabase) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdatabaseList) DeepCopyInto(out *FdatabaseList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Fdatabase, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdatabaseList.
func (in *FdatabaseList) DeepCopy() *FdatabaseList {
	if in == nil {
		return nil
	}
	out := new(FdatabaseList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *FdatabaseList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdatabaseSpec) DeepCopyInto(out *FdatabaseSpec) {
	*out = *in
	out.Database = in.Database
	out.User = in.User
	out.RootPassword = in.RootPassword
	out.Password = in.Password
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdatabaseSpec.
func (in *FdatabaseSpec) DeepCopy() *FdatabaseSpec {
	if in == nil {
		return nil
	}
	out := new(FdatabaseSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdatabaseStatus) DeepCopyInto(out *FdatabaseStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdatabaseStatus.
func (in *FdatabaseStatus) DeepCopy() *FdatabaseStatus {
	if in == nil {
		return nil
	}
	out := new(FdatabaseStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Fdeployment) DeepCopyInto(out *Fdeployment) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Fdeployment.
func (in *Fdeployment) DeepCopy() *Fdeployment {
	if in == nil {
		return nil
	}
	out := new(Fdeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Fdeployment) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdeploymentHealthCheck) DeepCopyInto(out *FdeploymentHealthCheck) {
	*out = *in
	out.LivenessProbe = in.LivenessProbe
	out.ReadinessProbe = in.ReadinessProbe
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdeploymentHealthCheck.
func (in *FdeploymentHealthCheck) DeepCopy() *FdeploymentHealthCheck {
	if in == nil {
		return nil
	}
	out := new(FdeploymentHealthCheck)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdeploymentList) DeepCopyInto(out *FdeploymentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Fdeployment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdeploymentList.
func (in *FdeploymentList) DeepCopy() *FdeploymentList {
	if in == nil {
		return nil
	}
	out := new(FdeploymentList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *FdeploymentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdeploymentResources) DeepCopyInto(out *FdeploymentResources) {
	*out = *in
	out.Requests = in.Requests
	out.Limits = in.Limits
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdeploymentResources.
func (in *FdeploymentResources) DeepCopy() *FdeploymentResources {
	if in == nil {
		return nil
	}
	out := new(FdeploymentResources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdeploymentSpec) DeepCopyInto(out *FdeploymentSpec) {
	*out = *in
	out.Resources = in.Resources
	out.HealthCheck = in.HealthCheck
	if in.Environments != nil {
		in, out := &in.Environments, &out.Environments
		*out = make([]Environment, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdeploymentSpec.
func (in *FdeploymentSpec) DeepCopy() *FdeploymentSpec {
	if in == nil {
		return nil
	}
	out := new(FdeploymentSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FdeploymentStatus) DeepCopyInto(out *FdeploymentStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FdeploymentStatus.
func (in *FdeploymentStatus) DeepCopy() *FdeploymentStatus {
	if in == nil {
		return nil
	}
	out := new(FdeploymentStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FromReference) DeepCopyInto(out *FromReference) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FromReference.
func (in *FromReference) DeepCopy() *FromReference {
	if in == nil {
		return nil
	}
	out := new(FromReference)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HealthProbe) DeepCopyInto(out *HealthProbe) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HealthProbe.
func (in *HealthProbe) DeepCopy() *HealthProbe {
	if in == nil {
		return nil
	}
	out := new(HealthProbe)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Resource) DeepCopyInto(out *Resource) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Resource.
func (in *Resource) DeepCopy() *Resource {
	if in == nil {
		return nil
	}
	out := new(Resource)
	in.DeepCopyInto(out)
	return out
}
