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

	// Group is a collection of DIJobs.
	Group string `json:"group,omitempty"`

	// Priority labels the priority of DIJob.
	// +kubebuilder:default=normal
	// +kubebuilder:validation:Enum=normal;high
	Priority Priority `json:"priority,omitempty"`

	// EngineFields defines features of the DI-engine framework.
	EngineFields EngineFields `json:"engineFields,omitempty"`

	// CleanPodPolicy defines the policy to clean pods after DIJob completed.
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

type EngineFields struct {
	// Topology defines the topology among the workers of the job.
	// +kubebuilder:default=star
	// +kubebuilder:validation:Enum=star;alone;mesh
	Topology Topology `json:"topology,omitempty"`

	// ParallelWorkers defines the number of parallel workers in each worker.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	ParallelWorkers int32 `json:"parallelWorkers,omitempty"`
}

// Priority defines the priority of DIJob
type Priority string

const (
	// PriorityNormal is normal priority
	PriorityNormal Priority = "normal"

	// PriorityHigh is high priority
	PriorityHigh Priority = "high"
)

type Topology string

const (
	// TopologyStar means all other workers are connected to a central node.
	TopologyStar Topology = "star"

	// TopologyAlone means no connections among workers.
	TopologyAlone Topology = "alone"

	// TopologyMesh means all workers are connected each other.
	TopologyMesh Topology = "mesh"
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

	// CompletionTimestamp defines the timestamp when the job was completed
	CompletionTimestamp *metav1.Time `json:"completionTimestamp,omitempty"`

	// Generation defines restart times of the job
	// +kubebuilder:default=0
	Generation int32 `json:"generation,omitempty"`

	// Phase defines the observed phase of the job
	Phase Phase `json:"phase,omitempty"`

	// Replicas defines the observed number of replicas of the job
	// +kubebuilder:default=0
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas defines the observed number of ready replicas of the job
	// +kubebuilder:default=0
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Allocation defines the replicas allocation of the job
	Allocation []string `json:"allocation,omitempty"`

	// Profilings defines the profiling data reported from DI-engine jobs
	Profilings Profilings `json:"profilings,omitempty"`

	// Conditions defines the conditions of the job
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
// +kubebuilder:printcolumn:name="Restarts",type=integer,JSONPath=`.status.generation`
// +kubebuilder:printcolumn:name="ReadyReplicas",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
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
