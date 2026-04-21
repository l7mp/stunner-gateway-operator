package lens

import (
	"fmt"
	"maps"
	"strings"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type DeploymentLens struct {
	appv1.Deployment `json:",inline"`
}

func NewDeploymentLens(d *appv1.Deployment) *DeploymentLens {
	return &DeploymentLens{Deployment: *d.DeepCopy()}
}

func (l *DeploymentLens) EqualResource(current client.Object) bool {
	d, ok := current.(*appv1.Deployment)
	if !ok {
		return false
	}

	cur := projectDeployment(d, &l.Deployment)
	des := projectDeployment(&l.Deployment, &l.Deployment)
	return apiequality.Semantic.DeepEqual(cur, des)
}

func (l *DeploymentLens) ApplyToResource(target client.Object) error {
	d, ok := target.(*appv1.Deployment)
	if !ok {
		return fmt.Errorf("deployment lens: invalid target type %T", target)
	}

	return applyDeployment(d, &l.Deployment)
}

func (l *DeploymentLens) EqualStatus(_ client.Object) bool {
	return true
}

func (l *DeploymentLens) ApplyToStatus(_ client.Object) error {
	return nil
}

func (l *DeploymentLens) DeepCopy() *DeploymentLens {
	return &DeploymentLens{Deployment: *l.Deployment.DeepCopy()}
}

func (l *DeploymentLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

// * * Deployment.ObjectMeta.Labels / Deployment.ObjectMeta.Annotations / Deployment.ObjectMeta.OwnerReferences
// * - renderer: sets deployment-level labels/annotations from Gateway + mandatory operator labels,
// *   and sets a singleton owner reference to the Gateway.
// * - updater: merges top-level labels/annotations and updates/creates the owner reference via
// *   setMetadata/addOwnerRef.
// *
// * * Deployment.Spec.Selector
// * - renderer: sets `{app=stunner,stunner.l7mp.io/related-gateway-name=<gw-name>,stunner.l7mp.io/related-gateway-namespace=<gw-namespace>}`.
// * - updater: deep-copies desired selector into current selector.
// *
// * * Deployment.Spec.Replicas
// * - renderer: sets only when Dataplane.Spec.Replicas is non-nil.
// * - updater: applies only when desired replicas is non-nil and != 1; otherwise preserves current
// *   replicas (autoscaler-friendly policy).
// *
// * * Deployment.Spec.Template.ObjectMeta.Labels / Deployment.Spec.Template.ObjectMeta.Annotations
// * - renderer: sets pod-template labels from Dataplane.Spec.Labels merged with operator pod labels,
// *   and pod-template annotations from Dataplane.Spec.Annotations merged with operator annotations.
// * - updater: overwrites pod-template labels/annotations from desired.
// *
// * * Deployment.Spec.Template.Spec.Containers
// * - renderer: initializes a stunner container and mutates image/command/args/env/resources/
// *   container-security-context/ports/liveness/readiness/imagePullPolicy from Dataplane policy.
// * - updater: deep-copies desired containers.
// *
// * * Deployment.Spec.Template.Spec.TerminationGracePeriodSeconds
// * - renderer: default base from config.TerminationGrace, overridden when
// *   Dataplane.Spec.TerminationGracePeriodSeconds is non-nil.
// * - updater: copies desired pointer when non-nil.
// *
// * * Deployment.Spec.Template.Spec.HostNetwork
// * - renderer: set from Dataplane.Spec.HostNetwork.
// * - updater: copies scalar.
// *
// * * Deployment.Spec.Template.Spec.Affinity
// * - renderer: set from Dataplane.Spec.Affinity when non-nil.
// * - updater: deep-copies when non-nil.
// *
// * * Deployment.Spec.Template.Spec.Tolerations
// * - renderer: set from Dataplane.Spec.Tolerations when non-nil.
// * - updater: deep-copies when non-nil.
// *
// * * Deployment.Spec.Template.Spec.SecurityContext
// * - renderer: set from Dataplane.Spec.SecurityContext when non-nil.
// * - updater: deep-copies when non-nil.
// *
// * * Deployment.Spec.Template.Spec.ImagePullSecrets
// * - renderer: set from Dataplane.Spec.ImagePullSecrets when non-empty.
// * - updater: deep-copies when non-empty.
// *
// * * Deployment.Spec.Template.Spec.TopologySpreadConstraints
// * - renderer: set from Dataplane.Spec.TopologySpreadConstraints when non-empty.
// * - updater: deep-copies when non-empty.
func applyDeployment(current, desired *appv1.Deployment) error {
	if err := setMetadata(current, desired); err != nil {
		return err
	}

	current.Spec.Selector = copyLabelSelector(desired.Spec.Selector)
	applyPodTemplateSpec(&current.Spec.Template, &desired.Spec.Template)

	if desired.Spec.Replicas != nil && int(*desired.Spec.Replicas) != 1 {
		replicas := *desired.Spec.Replicas
		current.Spec.Replicas = &replicas
	}

	return nil
}

func applyPodTemplateSpec(current, desired *corev1.PodTemplateSpec) {
	current.SetLabels(maps.Clone(desired.GetLabels()))
	current.SetAnnotations(maps.Clone(desired.GetAnnotations()))

	dpspec := &desired.Spec
	currentspec := &current.Spec

	currentspec.Containers = make([]corev1.Container, len(dpspec.Containers))
	for i := range dpspec.Containers {
		dpspec.Containers[i].DeepCopyInto(&currentspec.Containers[i])
	}

	if dpspec.TerminationGracePeriodSeconds != nil {
		grace := *dpspec.TerminationGracePeriodSeconds
		currentspec.TerminationGracePeriodSeconds = &grace
	}

	currentspec.HostNetwork = dpspec.HostNetwork

	if dpspec.Affinity != nil {
		currentspec.Affinity = dpspec.Affinity.DeepCopy()
	}

	if dpspec.Tolerations != nil {
		currentspec.Tolerations = make([]corev1.Toleration, len(dpspec.Tolerations))
		for i := range dpspec.Tolerations {
			dpspec.Tolerations[i].DeepCopyInto(&currentspec.Tolerations[i])
		}
	}

	if dpspec.SecurityContext != nil {
		currentspec.SecurityContext = dpspec.SecurityContext.DeepCopy()
	}

	if len(dpspec.ImagePullSecrets) != 0 {
		currentspec.ImagePullSecrets = make([]corev1.LocalObjectReference, len(dpspec.ImagePullSecrets))
		for i := range dpspec.ImagePullSecrets {
			dpspec.ImagePullSecrets[i].DeepCopyInto(&currentspec.ImagePullSecrets[i])
		}
	}

	if len(dpspec.TopologySpreadConstraints) != 0 {
		currentspec.TopologySpreadConstraints = make([]corev1.TopologySpreadConstraint, len(dpspec.TopologySpreadConstraints))
		for i := range dpspec.TopologySpreadConstraints {
			dpspec.TopologySpreadConstraints[i].DeepCopyInto(&currentspec.TopologySpreadConstraints[i])
		}
	}
}

func projectDeployment(d, owned *appv1.Deployment) *appv1.Deployment {
	src := d.DeepCopy()
	k8sscheme.Scheme.Default(src)

	ret := &appv1.Deployment{ObjectMeta: projectMetadata(src, owned)}
	ret.Spec.Selector = copyLabelSelector(src.Spec.Selector)
	ret.Spec.Replicas = normalizeReplicas(src.Spec.Replicas)
	ret.Spec.Template.ObjectMeta = projectTemplateMeta(&src.Spec.Template, &owned.Spec.Template)
	ret.Spec.Template.Spec = projectPodSpec(&src.Spec.Template.Spec)
	return ret
}

func normalizeReplicas(v *int32) *int32 {
	if v == nil {
		return nil
	}

	if *v == 1 {
		return nil
	}

	x := *v
	return &x
}

func copyLabelSelector(ls *metav1.LabelSelector) *metav1.LabelSelector {
	if ls == nil {
		return nil
	}

	return ls.DeepCopy()
}

func projectPodSpec(s *corev1.PodSpec) corev1.PodSpec {
	ret := corev1.PodSpec{
		TerminationGracePeriodSeconds: normalizeTerminationGracePeriodSeconds(s.TerminationGracePeriodSeconds),
		HostNetwork:                   s.HostNetwork,
		Affinity:                      s.Affinity.DeepCopy(),
		Tolerations:                   deepCopyTolerations(s.Tolerations),
		SecurityContext:               normalizePodSecurityContext(s.SecurityContext),
		ImagePullSecrets:              append([]corev1.LocalObjectReference(nil), s.ImagePullSecrets...),
		TopologySpreadConstraints:     deepCopyTopologySpread(s.TopologySpreadConstraints),
		Containers:                    make([]corev1.Container, 0, len(s.Containers)),
	}

	for i := range s.Containers {
		c := s.Containers[i]
		pc := corev1.Container{
			Name:            c.Name,
			Image:           c.Image,
			Command:         append([]string(nil), c.Command...),
			Args:            append([]string(nil), c.Args...),
			Ports:           projectContainerPorts(c.Ports),
			Env:             projectEnvVars(c.Env),
			SecurityContext: c.SecurityContext.DeepCopy(),
			Resources:       *c.Resources.DeepCopy(),
			LivenessProbe:   normalizeProbe(c.LivenessProbe),
			ReadinessProbe:  normalizeProbe(c.ReadinessProbe),
			ImagePullPolicy: normalizeImagePullPolicy(c.Image, c.ImagePullPolicy),
		}
		ret.Containers = append(ret.Containers, pc)
	}

	return ret
}

func normalizeTerminationGracePeriodSeconds(v *int64) *int64 {
	if v == nil {
		return nil
	}

	return copyInt64Ptr(v)
}

func normalizePodSecurityContext(v *corev1.PodSecurityContext) *corev1.PodSecurityContext {
	if v == nil {
		return nil
	}

	ret := v.DeepCopy()
	if apiequality.Semantic.DeepEqual(*ret, corev1.PodSecurityContext{}) {
		return nil
	}

	return ret
}

func normalizeProbe(p *corev1.Probe) *corev1.Probe {
	if p == nil {
		return nil
	}

	ret := p.DeepCopy()
	if ret.TimeoutSeconds == 0 {
		ret.TimeoutSeconds = 1
	}
	if ret.PeriodSeconds == 0 {
		ret.PeriodSeconds = 10
	}
	if ret.SuccessThreshold == 0 {
		ret.SuccessThreshold = 1
	}
	if ret.FailureThreshold == 0 {
		ret.FailureThreshold = 3
	}

	if ret.HTTPGet != nil && ret.HTTPGet.Scheme == "" {
		ret.HTTPGet.Scheme = corev1.URISchemeHTTP
	}

	return ret
}

func normalizeImagePullPolicy(image string, policy corev1.PullPolicy) corev1.PullPolicy {
	if policy != "" {
		return policy
	}

	if strings.Contains(image, "@") {
		return corev1.PullIfNotPresent
	}

	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	hasTag := colon > slash

	if !hasTag || strings.HasSuffix(image, ":latest") {
		return corev1.PullAlways
	}

	return corev1.PullIfNotPresent
}

func projectContainerPorts(ports []corev1.ContainerPort) []corev1.ContainerPort {
	if len(ports) == 0 {
		return nil
	}

	ret := make([]corev1.ContainerPort, len(ports))
	for i := range ports {
		ret[i] = corev1.ContainerPort{
			Name:          ports[i].Name,
			ContainerPort: ports[i].ContainerPort,
			Protocol:      ports[i].Protocol,
		}
	}

	return ret
}

func projectEnvVars(env []corev1.EnvVar) []corev1.EnvVar {
	if len(env) == 0 {
		return nil
	}

	ret := make([]corev1.EnvVar, len(env))
	for i := range env {
		ret[i] = corev1.EnvVar{
			Name:      env[i].Name,
			Value:     env[i].Value,
			ValueFrom: projectEnvVarSource(env[i].ValueFrom),
		}
	}

	return ret
}

func projectEnvVarSource(s *corev1.EnvVarSource) *corev1.EnvVarSource {
	if s == nil {
		return nil
	}

	v := &corev1.EnvVarSource{}
	if s.FieldRef != nil {
		v.FieldRef = &corev1.ObjectFieldSelector{FieldPath: s.FieldRef.FieldPath}
	}
	if s.ResourceFieldRef != nil {
		v.ResourceFieldRef = &corev1.ResourceFieldSelector{Resource: s.ResourceFieldRef.Resource}
	}
	if s.ConfigMapKeyRef != nil {
		v.ConfigMapKeyRef = s.ConfigMapKeyRef.DeepCopy()
	}
	if s.SecretKeyRef != nil {
		v.SecretKeyRef = s.SecretKeyRef.DeepCopy()
	}

	return v
}

func copyInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}

	x := *v
	return &x
}

func deepCopyTolerations(v []corev1.Toleration) []corev1.Toleration {
	if len(v) == 0 {
		return nil
	}

	ret := make([]corev1.Toleration, len(v))
	for i := range v {
		v[i].DeepCopyInto(&ret[i])
	}

	return ret
}

func deepCopyTopologySpread(v []corev1.TopologySpreadConstraint) []corev1.TopologySpreadConstraint {
	if len(v) == 0 {
		return nil
	}

	ret := make([]corev1.TopologySpreadConstraint, len(v))
	for i := range v {
		v[i].DeepCopyInto(&ret[i])
	}

	return ret
}
