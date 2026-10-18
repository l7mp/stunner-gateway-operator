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
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// RouteSpec defines the desired state of a STUNner route. It is shared by all route kinds
// (UDPRoute, TCPRoute) that forward traffic to a set of Kubernetes backends.
type RouteSpec struct {
	gwapiv1.CommonRouteSpec `json:",inline"`

	// Rules are a list of matchers and actions.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	Rules []RouteRule `json:"rules"`
}

// RouteRule is the configuration for a given rule.
type RouteRule struct {
	// BackendRefs defines the backend(s) where matching requests should be
	// sent. RouteRules correctly handle port ranges.
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

// convertBackendRefs converts the backend references of an upstream Gateway API route rule to the
// native representation. Every upstream route rule carries its refs as []gwapiv1.BackendRef, so
// this is the single place where the unsupported upstream fields are dropped: the backend port
// (the native optional port/endPort pair is a peer port-range filter, which has no official
// counterpart) and the weight.
func convertBackendRefs(refs []gwapiv1.BackendRef) []BackendRef {
	ret := make([]BackendRef, len(refs))
	for i := range refs {
		b := refs[i].BackendObjectReference
		ret[i].BackendObjectReference = BackendObjectReference{
			Group:     b.Group,
			Kind:      b.Kind,
			Name:      b.Name,
			Namespace: b.Namespace,
		}
	}
	return ret
}
