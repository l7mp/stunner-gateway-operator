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
	Containers []Container `json:"containers"`

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

// Note:
// - we cannot reuse corev1.Container and the rest of subresources because the resultant CRD does not validate:
// The CustomResourceDefinition "dataplanes.stunner.l7mp.io" is invalid: spec.validation.openAPIV3Schema.properties[spec].properties[containers].items.properties[resources].properties[claims].items.x-kubernetes-map-type: Invalid value: "null": must be atomic as item of a list with x-kubernetes-list-type=set
// - so we reproduce below what we need

type Container struct {
	// Name of the container specified as a DNS_LABEL.
	Name string `json:"name"`
	// Container image name.
	Image string `json:"image,omitempty"`
	// Entrypoint array. Not executed within a shell.
	// +optional
	Command []string `json:"command,omitempty"`
	// Arguments to the entrypoint.
	// +optional
	Args []string `json:"args,omitempty"`
	// List of ports to expose from the container.
	Ports []corev1.ContainerPort `json:"ports,omitempty"`
	// List of sources to populate environment variables in the container.
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`
	// List of environment variables to set in the container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// Compute Resources required by this container.
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty"`
	// Pod volumes to mount into the container's filesystem.
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
	// Periodic probe of container liveness.
	// +optional
	LivenessProbe *corev1.Probe `json:"livenessProbe,omitempty"`
	// Periodic probe of container service readiness.
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`
	// Image pull policy.
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// SecurityContext defines the security options the container should be run with.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty" protobuf:"bytes,15,opt,name=securityContext"`
}

type ResourceRequirements struct {
	// Limits describes the maximum amount of compute resources allowed.
	// +optional
	Limits corev1.ResourceList `json:"limits,omitempty"`
	// Requests describes the minimum amount of compute resources required.
	// +optional
	Requests corev1.ResourceList `json:"requests,omitempty"`
}

//+kubebuilder:object:root=true

// DataplaneList holds a list of static services.
type DataplaneList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// List of services.
	Items []Dataplane `json:"items"`
}

// helpers for converting between objects v1alpha1.X and corev1.X
// WARNING: this must be kept in sync with the Dataplane resource and subresource DeepCopy functions!

// DeepCopyInto is a deepcopy function, copying and transforming the receiver into a corev1 object.
func (in *Container) DeepCopyIntoCoreV1(out *corev1.Container) {
	out.Name = in.Name
	out.Image = in.Image
	if in.Command != nil {
		in, out := &in.Command, &out.Command
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Args != nil {
		in, out := &in.Args, &out.Args
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Ports != nil {
		in, out := &in.Ports, &out.Ports
		*out = make([]corev1.ContainerPort, len(*in))
		copy(*out, *in)
	}
	if in.EnvFrom != nil {
		in, out := &in.EnvFrom, &out.EnvFrom
		*out = make([]corev1.EnvFromSource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]corev1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Resources.DeepCopyIntoCoreV1(&out.Resources)

	out.VolumeMounts = make([]corev1.VolumeMount, len(in.VolumeMounts))
	for i := range in.VolumeMounts {
		in.VolumeMounts[i].DeepCopyInto(&out.VolumeMounts[i])
	}

	if in.LivenessProbe != nil {
		in, out := &in.LivenessProbe, &out.LivenessProbe
		*out = new(corev1.Probe)
		(*in).DeepCopyInto(*out)
	}
	if in.ReadinessProbe != nil {
		in, out := &in.ReadinessProbe, &out.ReadinessProbe
		*out = new(corev1.Probe)
		(*in).DeepCopyInto(*out)
	}
	out.ImagePullPolicy = in.ImagePullPolicy
	if in.SecurityContext != nil {
		in, out := &in.SecurityContext, &out.SecurityContext
		*out = new(corev1.SecurityContext)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopyToCoreV1 copues a v1alpha1.Container into a corev1.Container.
func (in *Container) DeepCopyToCoreV1() *corev1.Container {
	if in == nil {
		return nil
	}
	out := new(corev1.Container)
	in.DeepCopyIntoCoreV1(out)
	return out
}

// DeepCopyInto is a deepcopy function, copying and transforming the receiver into a corev1 object.
func (in *ResourceRequirements) DeepCopyIntoCoreV1(out *corev1.ResourceRequirements) {
	if in.Limits != nil {
		in, out := &in.Limits, &out.Limits
		*out = make(corev1.ResourceList, len(*in))
		for key, val := range *in {
			(*out)[key] = val.DeepCopy()
		}
	}
	if in.Requests != nil {
		in, out := &in.Requests, &out.Requests
		*out = make(corev1.ResourceList, len(*in))
		for key, val := range *in {
			(*out)[key] = val.DeepCopy()
		}
	}
}

// DeepCopyToCoreV1 copues a v1alpha1.ResourceRequirements into a corev1.ResourceRequirements.
func (in *ResourceRequirements) DeepCopyToCoreV1() *corev1.ResourceRequirements {
	if in == nil {
		return nil
	}
	out := new(corev1.ResourceRequirements)
	in.DeepCopyIntoCoreV1(out)
	return out
}
