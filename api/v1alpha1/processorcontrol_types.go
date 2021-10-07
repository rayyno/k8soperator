/*
Copyright 2020.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProcessorControlSpec defines the desired state of ProcessorControl
type ProcessorControlSpec struct {
	// the UUID of the parent process group where you want to deploy your dataflow, if not set deploy at root level.
	ProcessorControlID string `json:"processorControlID,omitempty"`
	// contains the reference to the NifiCluster with the one the processor is linked.
	ClusterRef ClusterReference `json:"clusterRef,omitempty"`
}

// ProcessorControlStatus defines the observed state of ProcessorControl
type ProcessorControlStatus struct {
	// process ID
	ProcessorID string `json:"processorID"`
	// the processor current state.
	State ProcessorControlState `json:"state"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ProcessorControl is the Schema for the processorcontrols API
type ProcessorControl struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProcessorControlSpec   `json:"spec,omitempty"`
	Status ProcessorControlStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ProcessorControlList contains a list of ProcessorControl
type ProcessorControlList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProcessorControl `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProcessorControl{}, &ProcessorControlList{})
}
