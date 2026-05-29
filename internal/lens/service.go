package lens

import (
	"fmt"
	"maps"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

type ServiceLens struct {
	corev1.Service `json:",inline"`
}

func NewServiceLens(s *corev1.Service) *ServiceLens {
	return &ServiceLens{Service: *s.DeepCopy()}
}

func (l *ServiceLens) EqualResource(current client.Object) bool {
	svc, ok := current.(*corev1.Service)
	if !ok {
		return false
	}

	return apiequality.Semantic.DeepEqual(projectService(svc, &l.Service), projectService(&l.Service, &l.Service))
}

func (l *ServiceLens) ApplyToResource(target client.Object) error {
	svc, ok := target.(*corev1.Service)
	if !ok {
		return fmt.Errorf("service lens: invalid target type %T", target)
	}

	if err := setMetadata(svc, &l.Service); err != nil {
		return err
	}

	projected := projectService(&l.Service, &l.Service)
	applyServiceSpec(svc, projected, &l.Service)

	return nil
}

func (l *ServiceLens) EqualStatus(_ client.Object) bool {
	return true
}

func (l *ServiceLens) ApplyToStatus(_ client.Object) error {
	return nil
}

func (l *ServiceLens) DeepCopy() *ServiceLens {
	return &ServiceLens{Service: *l.Service.DeepCopy()}
}

func (l *ServiceLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

// * Service.ObjectMeta.Labels / Service.ObjectMeta.Annotations / Service.ObjectMeta.OwnerReferences
// - renderer: starts from existing Service (if present), enforces operator mandatory metadata,
//   merges Gateway/GatewayConfig annotations, and sets Gateway owner reference.
// - updater: merges top-level labels/annotations and updates owner reference via setMetadata/addOwnerRef.
//
// * Service.Spec.Type
// - renderer: derived from service-type annotations with fallback to operator default.
// - updater: copied from projected desired.
//
// * Service.Spec.Selector
// - renderer: set according to dataplane mode (legacy: app=stunner, managed: related-gateway labels).
// - updater: copied from projected desired.
//
// * Service.Spec.SessionAffinity
// - renderer: set to ClientIP unless disable annotation requests None.
// - updater: copied from projected desired.
//
// * Service.Spec.ExternalTrafficPolicy
// - renderer: set to Local only when annotation requests it and type is NodePort/LoadBalancer,
//   otherwise left empty.
// - updater: copied from projected desired.
//
// * Service.Spec.Ports[].Name / Protocol / Port
// - renderer: built from valid Gateway listeners, merged with existing Service ports by name.
// - updater: copied from projected desired.
//
// * Service.Spec.Ports[].TargetPort
// - renderer: set from targetport annotations when present; otherwise left unset.
// - updater: projection normalizes zero TargetPort to Port to match API defaulting semantics.
//
// * Service.Spec.Ports[].NodePort
// - renderer: set from nodeport annotations when present.
// - updater: copied only when desired explicitly owns NodePorts; otherwise preserved from current.
//
// * Service.Spec.LoadBalancerIP
// - renderer: set from the first Gateway requested IP address when present.
// - updater: copied only when desired explicitly sets it; otherwise preserved from current.
//
// * Service.Spec.LoadBalancerClass
// - renderer: currently does not set.
// - updater: preserved from current (externally managed/immutable).

func projectService(s, owned *corev1.Service) *corev1.Service {
	src := s.DeepCopy()
	k8sscheme.Scheme.Default(src)

	ret := &corev1.Service{ObjectMeta: projectMetadata(src, owned)}
	ret.Spec.Type = src.Spec.Type
	ret.Spec.Selector = maps.Clone(src.Spec.Selector)
	ret.Spec.SessionAffinity = src.Spec.SessionAffinity
	ret.Spec.ExternalTrafficPolicy = normalizeExternalTrafficPolicy(src.Spec.Type, src.Spec.ExternalTrafficPolicy)
	ret.Spec.LoadBalancerIP = normalizeLoadBalancerIP(src, owned)
	ret.Spec.Ports = make([]corev1.ServicePort, 0, len(src.Spec.Ports))
	for i := range src.Spec.Ports {
		p := src.Spec.Ports[i]
		ret.Spec.Ports = append(ret.Spec.Ports, corev1.ServicePort{
			Name:       p.Name,
			Protocol:   p.Protocol,
			Port:       p.Port,
			TargetPort: normalizeTargetPort(p),
			NodePort:   normalizeNodePort(src, p),
		})
	}

	return ret
}

func applyServiceSpec(current, desired, owned *corev1.Service) {
	current.Spec.Type = desired.Spec.Type
	current.Spec.Selector = maps.Clone(desired.Spec.Selector)
	current.Spec.SessionAffinity = desired.Spec.SessionAffinity
	current.Spec.ExternalTrafficPolicy = desired.Spec.ExternalTrafficPolicy

	nextPorts := make([]corev1.ServicePort, len(desired.Spec.Ports))
	for i := range desired.Spec.Ports {
		desired.Spec.Ports[i].DeepCopyInto(&nextPorts[i])
		if !ownsNodePort(owned) {
			nextPorts[i].NodePort = getNodePortByName(current.Spec.Ports, nextPorts[i].Name)
		}
	}
	current.Spec.Ports = nextPorts

	if owned.Spec.LoadBalancerIP != "" {
		current.Spec.LoadBalancerIP = desired.Spec.LoadBalancerIP
	}
}

func normalizeLoadBalancerIP(svc, owned *corev1.Service) string {
	if owned.Spec.LoadBalancerIP == "" {
		return ""
	}

	return svc.Spec.LoadBalancerIP
}

func normalizeTargetPort(p corev1.ServicePort) intstr.IntOrString {
	t := p.TargetPort
	if t.Type == intstr.Int && t.IntVal == 0 && t.StrVal == "" {
		return intstr.FromInt32(p.Port)
	}

	return t
}

func normalizeExternalTrafficPolicy(svcType corev1.ServiceType,
	policy corev1.ServiceExternalTrafficPolicy) corev1.ServiceExternalTrafficPolicy {
	if policy == "" && (svcType == corev1.ServiceTypeNodePort || svcType == corev1.ServiceTypeLoadBalancer) {
		return corev1.ServiceExternalTrafficPolicyCluster
	}

	return policy
}

func normalizeNodePort(svc *corev1.Service, p corev1.ServicePort) int32 {
	if !ownsNodePort(svc) {
		return 0
	}

	return p.NodePort
}

func ownsNodePort(svc *corev1.Service) bool {
	_, ok := svc.GetAnnotations()[opdefault.NodePortAnnotationKey]
	return ok
}

func getNodePortByName(ports []corev1.ServicePort, name string) int32 {
	for i := range ports {
		if ports[i].Name == name {
			return ports[i].NodePort
		}
	}

	return 0
}
