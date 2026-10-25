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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func init() {
	SchemeBuilder.Register(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(GroupVersion, &UDPRoute{}, &UDPRouteList{})
		return nil
	})
}

// UDPRoute provides a way to route UDP traffic. When combined with a Gateway listener, it can be
// used to forward traffic on the port specified by the listener to a set of backends specified by
// the UDPRoute.
//
// Differences from Gateway API UDPRoutes
//   - port-ranges are correctly handled ([port, endPort])
//   - port is not mandatory
//   - backend weight is not supported
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=stunner
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type UDPRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of UDPRoute.
	Spec RouteSpec `json:"spec"`

	// Status defines the current state of UDPRoute.
	Status gwapiv1.UDPRouteStatus `json:"status,omitempty"`
}

// GetParentRefs returns the parent references of the route.
func (r *UDPRoute) GetParentRefs() []gwapiv1.ParentReference {
	return r.Spec.ParentRefs
}

// GetRules returns the rules of the route.
func (r *UDPRoute) GetRules() []RouteRule {
	return r.Spec.Rules
}

// GetRouteStatus returns the route status of the route.
func (r *UDPRoute) GetRouteStatus() *gwapiv1.RouteStatus {
	return &r.Status.RouteStatus
}

// +kubebuilder:object:root=true

// UDPRouteList contains a list of UDPRoute
type UDPRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UDPRoute `json:"items"`
}

// ConvertV1UDPRouteToStnrV1 converts a graduated Gateway API v1 UDPRoute to the STUNner-native
// representation, dropping the unsupported upstream backend weight (see
// convertBackendRefs).
func ConvertV1UDPRouteToStnrV1(src *gwapiv1.UDPRoute) *UDPRoute {
	if src == nil {
		return nil
	}
	dst := new(UDPRoute)
	src.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
	src.Spec.CommonRouteSpec.DeepCopyInto(&dst.Spec.CommonRouteSpec)
	src.Status.RouteStatus.DeepCopyInto(&dst.Status.RouteStatus)

	dst.Spec.Rules = make([]RouteRule, len(src.Spec.Rules))
	for i := range src.Spec.Rules {
		dst.Spec.Rules[i].BackendRefs = convertBackendRefs(src.Spec.Rules[i].BackendRefs)
	}
	return dst
}
