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
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func init() {
	SchemeBuilder.Register(&UDPRoute{}, &UDPRouteList{})
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
	Spec UDPRouteSpec `json:"spec"`

	// Status defines the current state of UDPRoute.
	Status gwapiv1a2.UDPRouteStatus `json:"status,omitempty"`
}

// UDPRouteSpec defines the desired state of UDPRoute.
type UDPRouteSpec struct {
	gwapiv1.CommonRouteSpec `json:",inline"`

	// Rules are a list of UDP matchers and actions.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Rules []UDPRouteRule `json:"rules"`
}

// UDPRouteRule is the configuration for a given rule.
type UDPRouteRule struct {
	// BackendRefs defines the backend(s) where matching requests should be
	// sent. UDPRouteRules correctly handle port ranges.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	BackendRefs []BackendRef `json:"backendRefs,omitempty"`
}

// BackendRef defines how a Route should forward a request to a Kubernetes resource.
type BackendRef struct {
	// BackendObjectReference references a Kubernetes object.
	BackendObjectReference `json:",inline"`
}

type BackendObjectReference struct {
	// Group is the group of the referent. For example, "gateway.networking.k8s.io".
	// When unspecified or empty string, core API group is inferred.
	//
	// +optional
	// +kubebuilder:default=""
	Group *gwapiv1.Group `json:"group,omitempty"`

	// Kind is the Kubernetes resource kind of the referent. For example
	// "Service".
	//
	// +optional
	// +kubebuilder:default=Service
	Kind *gwapiv1.Kind `json:"kind,omitempty"`

	// Name is the name of the referent.
	Name gwapiv1.ObjectName `json:"name"`

	// Namespace is the namespace of the backend. When unspecified, the local
	// namespace is inferred.
	//
	// +optional
	Namespace *gwapiv1.Namespace `json:"namespace,omitempty"`

	// Port specifies the destination port number to use for this resource. If port is not
	// specified, all ports are allowed. If port is defined but endPort is not, allow only
	// access to the given port. If both are specified, allows access in the port-range [port,
	// endPort] inclusive.
	//
	// +optional
	Port *gwapiv1.PortNumber `json:"port,omitempty"`

	// EndPort specifies the upper threshold of the port-range. Only considered of port is also specified.
	//
	// +optional
	EndPort *gwapiv1.PortNumber `json:"endPort,omitempty"`
}

// +kubebuilder:object:root=true

// UDPRouteList contains a list of UDPRoute
type UDPRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UDPRoute `json:"items"`
}

// conversions
func ConvertV1A2UDPRouteToV1(src *gwapiv1a2.UDPRoute) *UDPRoute {
	if src == nil {
		return nil
	}
	dst := new(UDPRoute)
	ConvertV1A2UDPRouteToV1Into(src, dst)
	return dst
}

func ConvertV1A2UDPRouteToV1Into(src *gwapiv1a2.UDPRoute, dst *UDPRoute) {
	src.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
	src.Spec.CommonRouteSpec.DeepCopyInto(&dst.Spec.CommonRouteSpec)
	src.Status.RouteStatus.DeepCopyInto(&dst.Status.RouteStatus)

	dst.Spec.Rules = make([]UDPRouteRule, len(src.Spec.Rules))
	for i := range src.Spec.Rules {
		dst.Spec.Rules[i].BackendRefs = make([]BackendRef, len(src.Spec.Rules[i].BackendRefs))
		for j := range src.Spec.Rules[i].BackendRefs {
			b := src.Spec.Rules[i].BackendRefs[j].BackendObjectReference
			dst.Spec.Rules[i].BackendRefs[j].BackendObjectReference = BackendObjectReference{
				Group:     b.Group,
				Kind:      b.Kind,
				Name:      b.Name,
				Namespace: b.Namespace,
				// ignore port!
			}
		}
	}
}

func ConvertV1UDPRouteToV1A2(src *UDPRoute) *gwapiv1a2.UDPRoute {
	if src == nil {
		return nil
	}
	dst := new(gwapiv1a2.UDPRoute)
	ConvertV1UDPRouteToV1A2Into(src, dst)
	return dst
}

func ConvertV1UDPRouteToV1A2Into(src *UDPRoute, dst *gwapiv1a2.UDPRoute) {
	src.ObjectMeta.DeepCopyInto(&dst.ObjectMeta)
	src.Spec.CommonRouteSpec.DeepCopyInto(&dst.Spec.CommonRouteSpec)
	src.Status.RouteStatus.DeepCopyInto(&dst.Status.RouteStatus)

	dst.Spec.Rules = make([]gwapiv1a2.UDPRouteRule, len(src.Spec.Rules))
	for i := range src.Spec.Rules {
		dst.Spec.Rules[i].BackendRefs = make([]gwapiv1a2.BackendRef, len(src.Spec.Rules[i].BackendRefs))
		for j := range src.Spec.Rules[i].BackendRefs {
			b := src.Spec.Rules[i].BackendRefs[j].BackendObjectReference
			dst.Spec.Rules[i].BackendRefs[j].BackendObjectReference = gwapiv1a2.BackendObjectReference{
				Group:     b.Group,
				Kind:      b.Kind,
				Name:      b.Name,
				Namespace: b.Namespace,
				// ignore port!
			}
		}
	}
}

func ConvertV1A2UDPRouteToV1List(src *gwapiv1a2.UDPRouteList) *UDPRouteList {
	if src == nil {
		return nil
	}
	dst := new(UDPRouteList)
	ConvertV1A2UDPRouteToV1ListInto(src, dst)
	return dst
}

func ConvertV1A2UDPRouteToV1ListInto(src *gwapiv1a2.UDPRouteList, dst *UDPRouteList) {
	dst.TypeMeta = src.TypeMeta
	src.ListMeta.DeepCopyInto(&dst.ListMeta)
	dst.Items = make([]UDPRoute, len(src.Items))
	for i := range src.Items {
		ConvertV1A2UDPRouteToV1Into(&src.Items[i], &dst.Items[i])
	}
}
