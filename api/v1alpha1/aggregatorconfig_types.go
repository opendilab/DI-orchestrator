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

// AggregatorConfigSpec defines the desired state of AggregatorConfig
type AggregatorConfigSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Aggregator AggregatorSpec `json:"aggregator,"`
}

//
type AggregatorSpec struct {
	Template corev1.PodTemplateSpec `json:"template,"`
}

// AggregatorConfigStatus defines the observed state of AggregatorConfig
type AggregatorConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Actors *AggregatorReplicaStatus `json:"actors,omitempty"`

	Learners *AggregatorReplicaStatus `json:"learners,omitempty"`
}

// AggregatorReplicaStatus defines the observed state of actors' and learners' replicas
type AggregatorReplicaStatus struct {
	Total int32 `json:"total,omitempty"`

	Active int32 `json:"active,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=agconfig

// AggregatorConfig is the Schema for the AggregatorConfigs API
type AggregatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec AggregatorConfigSpec `json:"spec,omitempty"`
	// Status AggregatorConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AggregatorConfigList contains a list of AggregatorConfig
type AggregatorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AggregatorConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AggregatorConfig{}, &AggregatorConfigList{})
}
