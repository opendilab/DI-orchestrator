/*


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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NervexJobSpec defines the desired state of NervexJob
type NervexJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Group is a collection of nervex jobs
	Group string `json:"group,omitempty"`

	//Priority labels the priority of nervex job
	PriorityClassName PriorityClassName `json:"priorityClassName,omitempty"`

	Coordinator CoordinatorSpec `json:"coordinator"`
}

// Priority defines the priority of nervex job
type PriorityClassName string

const (
	// NormalPriority is normal priority
	NormalPriority PriorityClassName = "default"

	// HighPriority is high priority
	HighPriority PriorityClassName = "high"
)

// CoordinatorSpec defines the desired state of coordinators
type CoordinatorSpec struct {
	Template corev1.PodTemplateSpec `json:"template"`
}

// NervexJobStatus defines the observed state of NervexJob
type NervexJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Phase Phase `json:"phase,omitempty"`

	Conditions []NervexJobCondition `json:"conditions,omitempty"`

	Actors int32 `json:"actors,omitempty"`

	Learners int32 `json:"learners,omitempty"`
}

// Phase defines the phase of NervexJob
type Phase string

const (
	// JobPending means the job has been submitted to the cluster,
	// but not all the pods and services have been created,
	// or not pods are running
	JobPending Phase = "Pending"

	// JobRunning means all the pods are in running state
	JobRunning Phase = "Running"

	// JobSucceeded means job completed without error
	JobSucceeded Phase = "Succeeded"

	// JobFailed means some pods failed, job is also considered failed
	JobFailed Phase = "Failed"

	// JobUnknown means the job is in unknown state
	JobUnknown Phase = "Unknown"
)

// NervexJobCondition records the conditions of NervexJob
type NervexJobCondition struct {
	// Type of job condition.
	Type JobConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// JobConditionType defines the condition state of NervexJob
type JobConditionType string

const (
	// PodCreated means all the pods of the job are created
	PodCreated JobConditionType = "PodCreated"

	// JobReady means the job is ready for training
	JobReady JobConditionType = "JobReady"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// NervexJob is the Schema for the nervexjobs API
type NervexJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NervexJobSpec   `json:"spec,omitempty"`
	Status NervexJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NervexJobList contains a list of NervexJob
type NervexJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NervexJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NervexJob{}, &NervexJobList{})
}
