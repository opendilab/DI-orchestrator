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

// DIJobSpec defines the desired state of DIJob
type DIJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Group is a collection of DIJobs
	Group string `json:"group,omitempty"`

	//Priority labels the priority of DIJob
	PriorityClassName PriorityClassName `json:"priorityClassName,omitempty"`

	// CleanPodPolicy defines the policy to clean pods after DIJob completed
	CleanPodPolicy CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	// Volumes defines the shared volumes for DI-engine components
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Coordinator defines the coordinator of distributed DIJob.
	// For serial DIJob, only coordinator is needed.
	// +kubebuilder:validation:Required
	Coordinator CoordinatorSpec `json:"coordinator"`

	// +kubebuilder:validation:Optional
	Collector CollectorSpec `json:"collector,"`

	// +kubebuilder:validation:Optional
	Learner LearnerSpec `json:"learner,"`
}

// Priority defines the priority of DIJob
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

	// CleanPodPolicyAll means deleting all pods of the job after completed
	CleanPodPolicyAll CleanPodPolicy = "All"

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

// DIJobStatus defines the observed state of DIJob
type DIJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Phase Phase `json:"phase,omitempty"`

	Conditions []DIJobCondition `json:"conditions,omitempty"`

	ReplicaStatus map[ReplicaType]*ReplicaStatus `json:"replicaStatus,omitempty"`
}

// Phase defines the phase of DIJob
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

// DIJobCondition records the conditions of DIJob
type DIJobCondition struct {
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
// +kubebuilder:resource:shortName=dijob
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DIJob is the Schema for the dijobs API
type DIJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DIJobSpec   `json:"spec,omitempty"`
	Status DIJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DIJobList contains a list of DIJob
type DIJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DIJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DIJob{}, &DIJobList{})
}
