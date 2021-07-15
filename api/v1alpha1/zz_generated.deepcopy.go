// +build !ignore_autogenerated

/*
Copyright 2021 The OpenDILab authors.

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

package v1alpha1

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AggregatorConfig) DeepCopyInto(out *AggregatorConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AggregatorConfig.
func (in *AggregatorConfig) DeepCopy() *AggregatorConfig {
	if in == nil {
		return nil
	}
	out := new(AggregatorConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AggregatorConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AggregatorConfigList) DeepCopyInto(out *AggregatorConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AggregatorConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AggregatorConfigList.
func (in *AggregatorConfigList) DeepCopy() *AggregatorConfigList {
	if in == nil {
		return nil
	}
	out := new(AggregatorConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *AggregatorConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AggregatorConfigSpec) DeepCopyInto(out *AggregatorConfigSpec) {
	*out = *in
	in.Aggregator.DeepCopyInto(&out.Aggregator)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AggregatorConfigSpec.
func (in *AggregatorConfigSpec) DeepCopy() *AggregatorConfigSpec {
	if in == nil {
		return nil
	}
	out := new(AggregatorConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AggregatorConfigStatus) DeepCopyInto(out *AggregatorConfigStatus) {
	*out = *in
	if in.Actors != nil {
		in, out := &in.Actors, &out.Actors
		*out = new(AggregatorReplicaStatus)
		**out = **in
	}
	if in.Learners != nil {
		in, out := &in.Learners, &out.Learners
		*out = new(AggregatorReplicaStatus)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AggregatorConfigStatus.
func (in *AggregatorConfigStatus) DeepCopy() *AggregatorConfigStatus {
	if in == nil {
		return nil
	}
	out := new(AggregatorConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AggregatorReplicaStatus) DeepCopyInto(out *AggregatorReplicaStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AggregatorReplicaStatus.
func (in *AggregatorReplicaStatus) DeepCopy() *AggregatorReplicaStatus {
	if in == nil {
		return nil
	}
	out := new(AggregatorReplicaStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AggregatorSpec) DeepCopyInto(out *AggregatorSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AggregatorSpec.
func (in *AggregatorSpec) DeepCopy() *AggregatorSpec {
	if in == nil {
		return nil
	}
	out := new(AggregatorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CollectorSpec) DeepCopyInto(out *CollectorSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CollectorSpec.
func (in *CollectorSpec) DeepCopy() *CollectorSpec {
	if in == nil {
		return nil
	}
	out := new(CollectorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CoordinatorSpec) DeepCopyInto(out *CoordinatorSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CoordinatorSpec.
func (in *CoordinatorSpec) DeepCopy() *CoordinatorSpec {
	if in == nil {
		return nil
	}
	out := new(CoordinatorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DIJob) DeepCopyInto(out *DIJob) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DIJob.
func (in *DIJob) DeepCopy() *DIJob {
	if in == nil {
		return nil
	}
	out := new(DIJob)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DIJob) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DIJobCondition) DeepCopyInto(out *DIJobCondition) {
	*out = *in
	in.LastUpdateTime.DeepCopyInto(&out.LastUpdateTime)
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DIJobCondition.
func (in *DIJobCondition) DeepCopy() *DIJobCondition {
	if in == nil {
		return nil
	}
	out := new(DIJobCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DIJobList) DeepCopyInto(out *DIJobList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DIJob, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DIJobList.
func (in *DIJobList) DeepCopy() *DIJobList {
	if in == nil {
		return nil
	}
	out := new(DIJobList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DIJobList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DIJobSpec) DeepCopyInto(out *DIJobSpec) {
	*out = *in
	if in.Volumes != nil {
		in, out := &in.Volumes, &out.Volumes
		*out = make([]v1.Volume, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Coordinator.DeepCopyInto(&out.Coordinator)
	in.Collector.DeepCopyInto(&out.Collector)
	in.Learner.DeepCopyInto(&out.Learner)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DIJobSpec.
func (in *DIJobSpec) DeepCopy() *DIJobSpec {
	if in == nil {
		return nil
	}
	out := new(DIJobSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DIJobStatus) DeepCopyInto(out *DIJobStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]DIJobCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ReplicaStatus != nil {
		in, out := &in.ReplicaStatus, &out.ReplicaStatus
		*out = make(map[ReplicaType]*ReplicaStatus, len(*in))
		for key, val := range *in {
			var outVal *ReplicaStatus
			if val == nil {
				(*out)[key] = nil
			} else {
				in, out := &val, &outVal
				*out = new(ReplicaStatus)
				**out = **in
			}
			(*out)[key] = outVal
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DIJobStatus.
func (in *DIJobStatus) DeepCopy() *DIJobStatus {
	if in == nil {
		return nil
	}
	out := new(DIJobStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *LearnerSpec) DeepCopyInto(out *LearnerSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new LearnerSpec.
func (in *LearnerSpec) DeepCopy() *LearnerSpec {
	if in == nil {
		return nil
	}
	out := new(LearnerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ReplicaStatus) DeepCopyInto(out *ReplicaStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ReplicaStatus.
func (in *ReplicaStatus) DeepCopy() *ReplicaStatus {
	if in == nil {
		return nil
	}
	out := new(ReplicaStatus)
	in.DeepCopyInto(out)
	return out
}
