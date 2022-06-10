package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:validation:Optional
// +kubebuilder:resource:shortName=gcfg,scope=Cluster
type GatewayConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec GatewayConfigSpec `json:"spec"`
}

type GatewayConfigSpec struct {
	Realm        string `json:"realm,omitempty"`
	AuthType     string `json:"authType,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	SharedSecret string `json:"sharedSecret,omitempty"`
	AuthLifetime *int32 `json:"authLifetime,omitempty"`
	Loglevel     string `json:"loglevel,omitempty"`
	MinPort      int32  `json:"minPort,omitempty"`
	MaxnPort     int32  `json:"maxPort,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// GatewayConfigList is a list of the GatewayConfig resources.
type GatewayConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []GatewayConfig `json:"items"`
}
