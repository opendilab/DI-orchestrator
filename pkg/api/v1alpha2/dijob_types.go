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

package v1alpha2

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
	// +kubebuilder:default=Running
	// +kubebuilder:validation:Enum=Running;All;None
	CleanPodPolicy CleanPodPolicy `json:"cleanPodPolicy,omitempty"`

	// Preemptible defines whether the dijob can be preempted.
	// +kubebuilder:default=false
	Preemptible bool `json:"preemptible,omitempty"`

	// MinReplicas defines the minimum number of replicas of DIJob.
	// +kubebuilder:validation:Minimum=0
	MinReplicas int32 `json:"minReplicas,omitempty"`

	// MaxReplicas defines the maximum number of replicas of DIJob.
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas,omitempty"`

	// Template defines the pod template for DIJob.
	// +kubebuilder:validation:Required
	Template corev1.PodTemplateSpec `json:"template"`
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

// DIJobStatus defines the observed state of DIJob
type DIJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Generation int32 `json:"generation,omitempty"`

	Phase Phase `json:"phase,omitempty"`

	Replicas int32 `json:"replicas,omitempty"`

	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	Allocation []string `json:"allocation,omitempty"`

	Profilings Profilings `json:"profilings,omitempty"`

	Conditions []DIJobCondition `json:"conditions,omitempty"`
}

// Phase defines the phase of DIJob
type Phase string

const (
	// JobPending means the job has been submitted to the cluster,
	// but not all the pods and services have been created,
	// or not pods are running
	JobPending Phase = "Pending"

	// JobStarted means the job has been scheduled and waits for running.
	JobStarting Phase = "Starting"

	// JobRestarting means the job has been rescheduled and waits for restarting.
	JobRestarting Phase = "Restarting"

	// JobRunning means all the pods are in running state
	JobRunning Phase = "Running"

	// JobSucceeded means job completed without error
	JobSucceeded Phase = "Succeeded"

	// JobFailed means some pods failed, job is also considered failed
	JobFailed Phase = "Failed"

	// JobUnknown means the job is in unknown state
	JobUnknown Phase = "Unknown"
)

type Profilings struct{}

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
