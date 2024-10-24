/*
Copyright 2024.

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

// VlanSpec defines the desired state of Vlan
type VlanSpec struct {
	// VlanCidr represents the CIDR range for the VLAN
	// +kubebuilder:validation:Pattern=`^([0-9]{1,3}\.){3}[0-9]{1,3}(/([0-9]|[1-2][0-9]|3[0-2]))$`
	VlanCidr     string          `json:"vlanCidr"`
	VlanLabel    string          `json:"vlanLabel"`
	ApiKeySecret SecretReference `json:"apiKeySecret"`
	Interval     metav1.Duration `json:"interval,omitempty"`
}

// VlanStatus defines the observed state of Vlan
type VlanStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Nodes []string `json:"nodes,omitempty"`
}

type SecretReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Vlan is the Schema for the vlans API
type Vlan struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VlanSpec   `json:"spec,omitempty"`
	Status VlanStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VlanList contains a list of Vlan
type VlanList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Vlan `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Vlan{}, &VlanList{})
}
