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

// StaticHostRoutesSpec defines the desired state of StaticHostRoutes
type StaticHostRoutesSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Routes   []StaticRoute   `json:"routes"`
	Interval metav1.Duration `json:"interval,omitempty"`
}

type StaticRoute struct {
	Cidr    string `json:"cidr"`
	Gateway string `json:"gateway"`
}

// StaticHostRoutesStatus defines the observed state of StaticHostRoutes
type StaticHostRoutesStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// StaticHostRoutes is the Schema for the statichostroutes API
type StaticHostRoutes struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StaticHostRoutesSpec   `json:"spec,omitempty"`
	Status StaticHostRoutesStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// StaticHostRoutesList contains a list of StaticHostRoutes
type StaticHostRoutesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StaticHostRoutes `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StaticHostRoutes{}, &StaticHostRoutesList{})
}
