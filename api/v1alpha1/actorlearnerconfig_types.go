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

// ActorLearnerConfigSpec defines the desired state of ActorLearnerConfig
type ActorLearnerConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Actor Actor `json:"actor,"`

	Learner Learner `json:"learner,"`

	Aggregator Aggregator `json:"aggregator,"`
}

// Actor defines the desired state of Actor
type Actor struct {
	Template corev1.PodTemplateSpec `json:"template,"`
}

// Learner defines the desired state of Learner
type Learner struct {
	Template corev1.PodTemplateSpec `json:"template,"`
}

//
type Aggregator struct {
	Template corev1.PodTemplateSpec `json:"template,"`
}

// ActorLearnerConfigStatus defines the observed state of ActorLearnerConfig
type ActorLearnerConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Actors *ReplicaStatus `json:"actors,omitempty"`

	Learners *ReplicaStatus `json:"learners,omitempty"`

	Aggregator *ReplicaStatus `json:"aggregator,omitempty"`
}

// ReplicaStatus defines the observed state of actors' and learners' replicas
type ReplicaStatus struct {
	Total int32 `json:"total,omitempty"`

	Active int32 `json:"active,omitempty"`

	Idle int32 `json:"idle,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=alconfig
// +kubebuilder:printcolumn:name="Total-Actors",type=integer,JSONPath=`.status.Actors.Total`
// +kubebuilder:printcolumn:name="Active-Actors",type=integer,JSONPath=`.status.Actors.Active`
// +kubebuilder:printcolumn:name="Total-Learners",type=integer,JSONPath=`.status.Learners.Total`
// +kubebuilder:printcolumn:name="Active-Learners",type=integer,JSONPath=`.status.Learners.Active`

// ActorLearnerConfig is the Schema for the ActorLearnerConfigs API
type ActorLearnerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ActorLearnerConfigSpec   `json:"spec,omitempty"`
	Status ActorLearnerConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ActorLearnerConfigList contains a list of ActorLearnerConfig
type ActorLearnerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ActorLearnerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ActorLearnerConfig{}, &ActorLearnerConfigList{})
}
