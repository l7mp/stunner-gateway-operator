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

func init() {
	SchemeBuilder.Register(&Dataplane{}, &DataplaneList{})
}

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

// this must be kept in sync with Renderer.createDeployment and Updater.upsertDeployment

// DataplaneSpec describes the prefixes reachable via a Dataplane.
type DataplaneSpec struct {
	// Number of desired pods. This is a pointer to distinguish between explicit zero and not
	// specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Container image name.
	// +optional
	Image string `json:"image,omitempty"`

	// Image pull policy. One of Always, Never, IfNotPresent.
	// +optional
	ImagePullPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Entrypoint array. Defaults: "stunnerd".
	// +optional
	Command []string `json:"command,omitempty"`

	// Arguments to the entrypoint.
	// +optional
	Args []string `json:"args,omitempty"`

	// Resources required by stunnerd.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Optional duration in seconds the stunnerd needs to terminate gracefully. Defaults to 3600 seconds.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// Host networking requested for the stunnerd pod to use the host's network namespace.
	// Can be used to implement public TURN servers with Kubernetes.  Defaults to false.
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// Scheduling constraints.
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
