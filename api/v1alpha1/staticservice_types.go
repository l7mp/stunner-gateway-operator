/*
Copyright 2022 The l7mp/stunner team.

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

//+kubebuilder:object:root=true
// //+kubebuilder:subresource:status
//+kubebuilder:resource:categories=stunner,shortName=ssvc

// StaticService is a set of static IP address prefixes STUNner allows access to via a Route. The
// purpose is to allow a Service-like CRD containing a set of static IP address prefixes to be set
// as the backend of a UDPRoute (or TCPRoute).
type StaticService struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the behavior of a service.
	Spec StaticServiceSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

// StaticServiceSpec describes the prefixes reachable via a StaticService.
type StaticServiceSpec struct {
	// The list of ports reachable via this service (currently omitted).
	// +patchMergeKey=port
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=port
	// +listMapKey=protocol
	// +optional
	Ports []corev1.ServicePort `json:"ports,omitempty" patchStrategy:"merge" patchMergeKey:"port" protobuf:"bytes,1,rep,name=ports"`

	// Prefixes is a list of IP address prefixes reachable via this route.
	Prefixes []string `json:"prefixes"`
}

//+kubebuilder:object:root=true

// StaticServiceList holds a list of static services.
type StaticServiceList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of services.
	Items []StaticService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StaticService{}, &StaticServiceList{})
}
