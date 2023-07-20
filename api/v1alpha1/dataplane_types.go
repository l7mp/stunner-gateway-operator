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
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Dataplane{}, &DataplaneList{})
}

//+kubebuilder:object:root=true
// //+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:resource:categories=stunner,shortName=dps

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

// DataplaneSpec describes the prefixes reachable via a Dataplane.
type DataplaneSpec struct {
	// from apps/v1.DeploymentSpec

	// Number of desired pods. This is a pointer to distinguish between explicit zero and not
	// specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// The deployment strategy to use to replace existing stunnerd pods with new ones.
	// +optional
	Strategy *appv1.DeploymentStrategy `json:"strategy,omitempty"`

	// from v1.PodTemplateSpec

	// List of containers belonging to the pod. Must contain at least one container template
	// for stunnerd. Can be used to influence the stunnerd container image repo and version or
	// inject sidecars next to stunnerd.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []corev1.Container `json:"containers"`

	// List of volumes that can be mounted by containers belonging to the pod.
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge,retainKeys
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Optional duration in seconds the stunnerd needs to terminate gracefully.  Value must be
	// non-negative integer. The value zero indicates stop immediately via the kill signal (no
	// opportunity to shut down).  If this value is nil, the default grace period will be used
	// instead.  The grace period is the duration in seconds after the stunnerd process running
	// is sent a termination signal and the time when it is forcibly halted with a kill signal,
	// disconnecting all TURN allocations running on the pod.  Set this value longer than the
	// expected cleanup time for your clients.  Defaults to 600 seconds.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// Host networking requested for the stunnerd pod to use the host's network namespace.
	// Can be used to implement public TURN servers with Kubernetes.  Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// If specified, the stunnerd pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
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
