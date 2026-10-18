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
		scheme.AddKnownTypes(GroupVersion, &TURNRoute{}, &TURNRouteList{})
		return nil
	})
}

// TURNRoute routes the clients of a Gateway listener to their peers through an upstream TURN
// server. On a TURN listener it relays the allocations of the clients through the upstream TURN
// server, admitting every peer; on a plain (L4) listener it relays each client connection to a
// backend of the route through the upstream TURN server.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=stunner
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="URI",type=string,JSONPath=`.spec.tunnel.uri`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type TURNRoute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of TURNRoute.
	Spec TURNRouteSpec `json:"spec"`

	// Status defines the current state of TURNRoute.
	Status gwapiv1.RouteStatus `json:"status,omitempty"`
}

// TURNRouteSpec defines the desired state of a TURNRoute.
type TURNRouteSpec struct {
	gwapiv1.CommonRouteSpec `json:",inline"`

	// Rules are the rules of the route: their backends are the peers a plain (L4) listener dials
	// through the upstream TURN server, at the port of the backend. A TURN listener does not use
	// them.
	//
	// +optional
	Rules []RouteRule `json:"rules,omitempty"`

	// Tunnel is the upstream TURN server the peers are reached through.
	Tunnel TURNTunnel `json:"tunnel"`
}

// TURNTunnel is an upstream TURN server and the credentials to reach it.
type TURNTunnel struct {
	// URI is the upstream TURN server as an RFC 7065 TURN URI, e.g.,
	// "turn:turn.example.com:3478?transport=udp" or "turns:turn.example.com:443?transport=tcp".
	//
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`

	// AuthRef references the Secret holding the credentials presented to the upstream TURN
	// server: the key "type" is "static" (the default) or "ephemeral", the optional keys "realm"
	// and "lifetime" set the realm and the lifetime of generated ephemeral credentials, and every
	// other key is a credential ("username" and "password" for static, "secret" for ephemeral
	// authentication). No reference means no authentication.
	//
	// +optional
	AuthRef *gwapiv1.SecretObjectReference `json:"authRef,omitempty"`

	// Insecure skips the verification of the certificate of the upstream TURN server over TLS and
	// DTLS.
	//
	// +optional
	Insecure bool `json:"insecure,omitempty"`

	// SNI overrides the server name the certificate of the upstream TURN server is verified
	// against.
	//
	// +optional
	SNI string `json:"sni,omitempty"`
}

// GetParentRefs returns the parent references of the route.
func (r *TURNRoute) GetParentRefs() []gwapiv1.ParentReference {
	return r.Spec.ParentRefs
}

// GetRouteStatus returns the route status of the route.
func (r *TURNRoute) GetRouteStatus() *gwapiv1.RouteStatus {
	return &r.Status
}

// GetRules returns the rules of the route.
func (r *TURNRoute) GetRules() []RouteRule {
	return r.Spec.Rules
}

// +kubebuilder:object:root=true

// TURNRouteList holds a list of TURNRoutes.
type TURNRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TURNRoute `json:"items"`
}
