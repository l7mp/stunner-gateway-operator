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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

// GatewayConfigSpec defines the desired state of GatewayConfig
type GatewayConfigSpec struct {
	// StunnerConfig specifies the name of the ConfigMap into which the operator renders the
	// stunnerd configfile.
	//
	// +optional
	// +kubebuilder:validation:MaxLength=64
	// +kubebuilder:validation:Pattern=`^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
	// +kubebuilder:default:="stunnerd-config"
	StunnerConfig *string `json:"stunnerConfig,omitempty"`

	// Realm defines the STUN/TURN authentication realm to be used for clients toauthenticate
	// with STUNner.
	//
	// The realm must consist of lower case alphanumeric characters or '-', and must start and
	// end with an alphanumeric character. No other punctuation is allowed.
	//
	// +optional
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +kubebuilder:default:="stunner.l7mp.io"
	Realm *string `json:"realm,omitempty"`

	// MetricsEndpoint is the URI in the form `http://address:port/path` exposed for metric
	// scraping (Prometheus). The scheme (`http://`) is mandatory. Default is to expose no
	// metric endpoint.
	//
	// +optional
	MetricsEndpoint *string `json:"metricsEndpoint,omitempty"`

	// HealthCheckEndpoint is the URI of the form `http://address:port` exposed for external
	// HTTP health-checking. A liveness probe responder will be exposed on path `/live` and
	// readiness probe on path `/ready`. The scheme (`http://`) is mandatory, default is to
	// enable health-checking at "http://0.0.0.0:8086".
	//
	// +optional
	HealthCheckEndpoint *string `json:"healthCheckEndpoint,omitempty"`

	// AuthType is the type of the STUN/TURN authentication mechanism.
	//
	// +optional
	// +kubebuilder:validation:Pattern=`^plaintext|longterm$`
	// +kubebuilder:default:="plaintext"
	AuthType *string `json:"authType,omitempty"`

	// Username defines the `username` credential for "plaintext" authentication.
	//
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9!#$%&'*+\-.^_\x60|~]+$`
	Username *string `json:"userName,omitempty"`

	// Password defines the `password` credential for "plaintext" authentication.
	//
	// +optional
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9!#$%&'*+\-.^_\x60|~]+$`
	Password *string `json:"password,omitempty"`

	// SharedSecret defines the shared secret to be used for "longterm" authentication.
	//
	// +optional
	SharedSecret *string `json:"sharedSecret,omitempty"`

	// AuthLifetime defines the lifetime of "longterm" authentication credentials in seconds.
	//
	// +optional
	AuthLifetime *int32 `json:"authLifetime,omitempty"`

	// AuthRef holds an optional reference to a Secret that specifies the TURN authentication
	// credentials for STUNner.  The following conditions must hold:
	// - group MUST be set to "" (corev1.GroupName), "v1", or omitted,
	// - kind MUST be set to "Secret" or omitted,
	// - name MUST be the name of a valid Secret,
	// - namespace MAY be omitted, in which case it defaults to the namespace of
	//   the GatewayConfig, or it MAY be any valid namespace where the Secret lives.

	// The referenced Secret MUST be of type Opaque and the following conditions MUST hold:
	// - the Secret MUST contain a "type" field that MUST be set to either
	//   "plaintext" or "longterm" (but see
	//   https://github.com/l7mp/stunner-gateway-operator/issues/7),
	// - if type is "plaintext" then the Secret MUST contain a "username" and a
	//   "password" field that together specify the username/password pair clients can use to
	//   authenticate with Stunner,
	// - if type is "lonterm" then the Secret MUST contain a single field named
	//   "sharedSecret" or "secret" that is used to check the authenticity of time-windowed
	//   TURN credentials.

	// Note that externally set credentials override any inline auth credentials (AuthType,
	// AuthUsername, etc.): if AuthRef is nonempty then it is expected that the referenced
	// Secret exists and *all* authentication credentials are correctly set in the referenced
	// Secret (username/password or shared secret). Mixing of credential sources
	// (inline/external) is not supported.
	//
	// +optional
	AuthRef *gwapiv1b1.SecretObjectReference `json:"authRef,omitempty"`

	// LoadBalancerServiceAnnotations is a list of annotations that will go into the
	// LoadBalancer services created automatically by the operator to wrap Gateways.
	//
	// NOTE: removing annotations from a GatewayConfig will not result in the removal of the
	// corresponding annotations from the LoadBalancer service, in order to prevent the
	// accidental removal of an annotation installed there by Kubernetes or the cloud
	// provider. If you really want to remove an annotation, do this manually or simply remove
	// all Gateways (which will remove the corresponding LoadBalancer services), update the
	// GatewayConfig and then recreate the Gateways, so that the newly created LoadBalancer
	// services will contain the required annotations.
	//
	// +optional
	LoadBalancerServiceAnnotations map[string]string `json:"loadBalancerServiceAnnotations,omitempty"`

	// LogLevel specifies the default loglevel for the STUNner daemon.
	//
	// +optional
	LogLevel *string `json:"logLevel,omitempty"`

	// MinRelayPort is the smallest relay port assigned for STUNner relay connections.
	//
	// +optional
	// +kubebuilder:validation:Minimum:1
	// +kubebuilder:validation:Maximum:65535
	MinPort *int32 `json:"minPort,omitempty"`

	// MaxRelayPort is the smallest relay port assigned for STUNner relay connections.
	//
	// +kubebuilder:validation:Minimum:1
	// +kubebuilder:validation:Maximum:65535
	MaxPort *int32 `json:"maxPort,omitempty"`
}

//+kubebuilder:object:root=true
// //+kubebuilder:subresource:status
//+kubebuilder:resource:categories=stunner,shortName=gtwconf
//+kubebuilder:printcolumn:name="Realm",type=string,JSONPath=`.spec.realm`
//+kubebuilder:printcolumn:name="Auth",type=string,JSONPath=`.spec.authType`
//+kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// GatewayConfig is the Schema for the gatewayconfigs API
type GatewayConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GatewayConfigSpec `json:"spec,omitempty"`
	// Status GatewayConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayConfigList contains a list of GatewayConfig
type GatewayConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayConfig{}, &GatewayConfigList{})
}
