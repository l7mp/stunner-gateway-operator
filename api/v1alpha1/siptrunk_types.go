/*
Copyright 2026 The l7mp/stunner team.

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
	"k8s.io/apimachinery/pkg/runtime"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func init() {
	SchemeBuilder.Register(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(GroupVersion, &SIPTrunk{}, &SIPTrunkList{})
		return nil
	})
}

// SIPTrunk names the listener of a Gateway carrying the media, and tells how the carriers
// connect. SIP Gateways can reference a SIPTrunk in spec.infrastructure.parametersRef.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=stunner
// +kubebuilder:printcolumn:name="Media",type=string,JSONPath=`.spec.mediaListener`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type SIPTrunk struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec SIPTrunkSpec `json:"spec,omitempty"`
}

// SIPTrunkSpec is the spec of a SIPTrunk.
type SIPTrunkSpec struct {
	// MediaListener is the name of the Gateway listener carrying the media of the calls. Every
	// other listener of the Gateway is a SIP listener.
	MediaListener gwapiv1.SectionName `json:"mediaListener"`

	// AllowedSources are the address prefixes the SIP listeners admit calls from. Empty admits no
	// inbound calls, which suits registration-only trunks.
	//
	// +optional
	AllowedSources []string `json:"allowedSources,omitempty"`

	// Registrations are the registrations the gateway keeps up with registration trunks.
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Registrations []SIPRegistration `json:"registrations,omitempty"`
}

// SIPRegistration is a registration of an address of record with a carrier's registrar.
type SIPRegistration struct {
	// Name of the registration, unique in the SIPTrunk and distinct from the listener names of
	// the Gateway. A route attaches to the registration by this name as its sectionName: its
	// backends take the calls of the registration, so the name is a valid sectionName.
	//
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	Name string `json:"name"`

	// Registrar is the SIP URI of the carrier's registrar, as Kamailio or Asterisk names one:
	// sip:host[:port][;transport=udp|tcp|tls], or sips:host[:port] for TLS; the scheme may be
	// omitted for sip:. The transport defaults to UDP, TLS for sips:, the port to 5060, 5061 over
	// TLS.
	//
	// +kubebuilder:validation:MinLength=1
	Registrar string `json:"registrar"`

	// AOR is the address of record to register, a sip: or sips: URI.
	//
	// +kubebuilder:validation:MinLength=1
	AOR string `json:"aor"`

	// AuthRef names a Secret in the namespace of the SIPTrunk whose username and password keys
	// answer the registrar's digest challenge.
	//
	// +optional
	AuthRef *corev1.LocalObjectReference `json:"authRef,omitempty"`

	// Expires is the registration lifetime the gateway asks for.
	//
	// +optional
	// +kubebuilder:default="600s"
	Expires *metav1.Duration `json:"expires,omitempty"`
}

// SIPTrunkList holds a list of SIPTrunks.
//
// +kubebuilder:object:root=true
type SIPTrunkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SIPTrunk `json:"items"`
}
