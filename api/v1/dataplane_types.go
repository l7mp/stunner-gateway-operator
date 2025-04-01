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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Hub marks Dataplane.v1 as a conversion hub.
func (*Dataplane) Hub() {}

func init() {
	SchemeBuilder.Register(&Dataplane{}, &DataplaneList{})
}

type DataplaneResourceType string

const (
	DataplaneResourceDeployment DataplaneResourceType = "Deployment"
	DataplaneResourceDaemonSet  DataplaneResourceType = "DaemonSet"
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:resource:categories=stunner,scope=Cluster,shortName=dps
// +kubebuilder:storageversion

// Dataplane is a collection of configuration parameters that can be used for spawning a `stunnerd`
// instance for a Gateway. Labels and annotations on the Dataplane object will be copied verbatim
// into the target Deployment.
type Dataplane struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the behavior of a Dataplane resource.
	Spec DataplaneSpec `json:"spec,omitempty"`
}

// this must be kept in sync with Renderer.createDeployment and generateDaemonSet, as well as
// Updater.upsertDeployment and Updater.upsertDaemonSet

// DataplaneSpec describes the prefixes reachable via a Dataplane.
type DataplaneSpec struct {
	// Container image name.
	//
	// +optional
	Image string `json:"image,omitempty"`

	// Image pull policy. One of Always, Never, IfNotPresent.
	//
	// +optional
	ImagePullPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// ImagePullSecrets is an optional list of references to secrets to use for pulling the
	// stunnerd image. Note that the referenced secrets are not watched by the operator, so
	// modifications will in effect only for newly created pods. Also note that the Secret is
	// always searched in the same namespace as the Gateway, which allows to use separate pull
	// secrets per each namespace.
	//
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// DataplaneResource defines the Kubernetes resource kind to use to deploy the dataplane,
	// can be either Deployment (default) or DaemonSet (supported only in the premium tier).
	//
	// +optional
	// +kubebuilder:default=Deployment
	// +kubebuilder:validation:Enum="Deployment";"DaemonSet"
	DataplaneResource *DataplaneResourceType `json:"dataplaneResource,omitempty"`

	// Custom labels to add to dataplane pods. Note that this does not affect the labels added
	// to the dataplane resource (Deployment or DaemonSet) as those are copied from the
	// Gateway, just the pods. Note also that mandatory pod labels override whatever you set
	// here on conflict. The only way to set pod labels is here: whatever you set manually on
	// the dataplane pod will be reset by the opetator.
	//
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Custom annotations to add to dataplane pods. Note that this does not affect the
	// annotations added to the dataplane resource (Deployment or DaemonSet) as those are
	// copied from the correspnding Gateway, just the pods. Note also that mandatory pod
	// annotations override whatever you set here on conflict, and the annotations set here
	// override annotations manually added to the pods.
	//
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Entrypoint array. Defaults: "stunnerd".
	//
	// +optional
	Command []string `json:"command,omitempty"`

	// Arguments to the entrypoint.
	//
	// +optional
	Args []string `json:"args,omitempty"`

	// List of sources to populate environment variables in the stunnerd container.
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// List of environment variables to set in the stunnerd container.
	//
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// ContainerSecurityContext holds container-level security attributes specifically for the
	// stunnerd container.
	//
	// +optional
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`

	// Number of desired pods. If empty or set to 1, use whatever is in the target Deployment,
	// otherwise overwite whatever is in the Deployment (this may block autoscaling the
	// dataplane though). Ignored if the dataplane is deployed into a DaemonSet. Defaults to 1.
	//
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources required by stunnerd.
	//
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Optional duration in seconds the stunnerd needs to terminate gracefully. Defaults to 3600 seconds.
	//
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// Host networking requested for the stunnerd pod to use the host's network namespace.
	// Can be used to implement public TURN servers with Kubernetes.  Defaults to false.
	//
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// Scheduling constraints.
	//
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// SecurityContext holds pod-level security attributes and common container settings.
	//
	// +optional
	SecurityContext *corev1.PodSecurityContext `json:"securityContext,omitempty"`

	// If specified, the pod's tolerations.
	//
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// TopologySpreadConstraints describes how stunnerd pods ought to spread across topology
	// domains.
	//
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// Disable health-checking. Default is to enable HTTP health-checks on port 8086: a
	// liveness probe responder will be exposed on path `/live` and readiness probe on path
	// `/ready`.
	//
	// +optional
	DisableHealthCheck bool `json:"disableHealthCheck,omitempty"`

	// EnableMetricsEnpoint can be used to enable metrics scraping (Prometheus). If enabled, a
	// metrics endpoint will be available at http://0.0.0.0:8080 at all dataplane pods. Default
	// is no metrics collection.
	//
	// +optional
	EnableMetricsEnpoint bool `json:"enableMetricsEndpoint,omitempty"`

	// OffloadEngine defines the dataplane offload mode, either "None", "XDP", "TC", or
	// "Auto". Set to "Auto" to let STUNner find the optimal offload mode. Default is "None".
	//
	// +optional
	// +kubebuilder:default=None
	// +kubebuilder:validation:Pattern=`^None|XDP|TC|Auto$`
	OffloadEngine string `json:"offload_engine,omitempty"`

	// OffloadInterfaces explicitly specifies the interfaces on which to enable the offload
	// engine. Empty list means to enable offload on all interfaces (this is the default).
	//
	// +optional
	OffloadInterfaces []string `json:"offload_interfaces,omitempty"`
}

// +kubebuilder:object:root=true

// DataplaneList holds a list of static services.
type DataplaneList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of services.
	Items []Dataplane `json:"items"`
}
