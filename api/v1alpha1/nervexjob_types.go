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

// NerveXJobSpec defines the desired state of NerveXJob
type NerveXJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Group is a collection of NerveXJobs
	Group string `json:"group,omitempty"`

	//Priority labels the priority of NerveXJob
	PriorityClassName PriorityClassName `json:"priorityClassName,omitempty"`

	// CleanPodPolicy defines the policy to clean pods after NerveXJob completed
	CleanPodPolicy CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	Coordinator CoordinatorSpec `json:"coordinator"`

	Collector CollectorSpec `json:"collector,"`

	Learner LearnerSpec `json:"learner,"`
}

// Priority defines the priority of NerveXJob
type PriorityClassName string

const (
	// NormalPriority is normal priority
	NormalPriority PriorityClassName = "default"

	// HighPriority is high priority
	HighPriority PriorityClassName = "high"
)

type CleanPodPolicy string

const (
	// CleanPodPolicyRunning means deleting all running pods of the job after completed
	CleanPodPolicyRunning CleanPodPolicy = "Running"

	// CleanPodPolicyALL means deleting all pods of the job after completed
	CleanPodPolicyALL CleanPodPolicy = "ALL"

	// CleanPodPolicyNone means never deleting any pods of the job after completed
	CleanPodPolicyNone CleanPodPolicy = "None"
)

// CoordinatorSpec defines the desired state of coordinators
type CoordinatorSpec struct {
	Template corev1.PodTemplateSpec `json:"template"`
}

// CollectorSpec defines the desired state of CollectorSpec
type CollectorSpec struct {
	Template corev1.PodTemplateSpec `json:"template,"`
}

// Learner defines the desired state of Learner
type LearnerSpec struct {
	Template corev1.PodTemplateSpec `json:"template,"`
}

// NerveXJobStatus defines the observed state of NerveXJob
type NerveXJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Phase Phase `json:"phase,omitempty"`

	Conditions []NerveXJobCondition `json:"conditions,omitempty"`

	ReplicaStatus map[ReplicaType]*ReplicaStatus `json:"replicaStatus,omitempty"`
}

// Phase defines the phase of NerveXJob
type Phase string

const (
	// JobCreated means the job has been submitted to the cluster,
	// but not all the pods and services have been created,
	// or not pods are running
	JobCreated Phase = "Created"

	// JobRunning means all the pods are in running state
	JobRunning Phase = "Running"

	// JobSucceeded means job completed without error
	JobSucceeded Phase = "Succeeded"

	// JobFailed means some pods failed, job is also considered failed
	JobFailed Phase = "Failed"

	// JobUnknown means the job is in unknown state
	JobUnknown Phase = "Unknown"
)

// ReplicaType represents the type of the replica. Each operator needs to define its
// own set of ReplicaTypes.
type ReplicaType string

const (
	ReplicaTypeCollector   ReplicaType = "Collector"
	ReplicaTypeLearner     ReplicaType = "Learner"
	ReplicaTypeAggregator  ReplicaType = "Aggregator"
	ReplicaTypeCoordinator ReplicaType = "Coordinator"
)

// ReplicaStatus represents the current observed state of the replica.
type ReplicaStatus struct {
	// The number of actively running pods.
	Active int32 `json:"active,omitempty"`

	// The number of pods which reached phase Succeeded.
	Succeeded int32 `json:"succeeded,omitempty"`

	// The number of pods which reached phase Failed.
	Failed int32 `json:"failed,omitempty"`
}

// NerveXJobCondition records the conditions of NerveXJob
type NerveXJobCondition struct {
	// Type of job condition.
	Type Phase `json:"type"`
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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=nvxjob
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NerveXJob is the Schema for the nervexjobs API
type NerveXJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NerveXJobSpec   `json:"spec,omitempty"`
	Status NerveXJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NerveXJobList contains a list of NerveXJob
type NerveXJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NerveXJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NerveXJob{}, &NerveXJobList{})
}
