package lens

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceEqualNormalizesDefaults(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:                          corev1.ServiceTypeLoadBalancer,
			ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyCluster,
			Selector:                      map[string]string{"app": "stunner"},
			AllocateLoadBalancerNodePorts: ptrBool(true),
			Ports: []corev1.ServicePort{{
				Name:       "udp",
				Protocol:   corev1.ProtocolUDP,
				Port:       3478,
				TargetPort: intstr.FromInt(3478),
			}},
		},
	}

	candidate := current.DeepCopy()
	candidate.Spec.ExternalTrafficPolicy = ""
	candidate.Spec.AllocateLoadBalancerNodePorts = nil
	candidate.Spec.Ports[0].TargetPort = intstr.IntOrString{}

	v := NewServiceLens(candidate)
	if !v.EqualResource(current) {
		t.Fatalf("expected service lenses to match after default normalization")
	}
}

func TestServiceEqualDetectsRealDiff(t *testing.T) {
	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": "stunner"},
			Ports:    []corev1.ServicePort{{Name: "udp", Port: 3478, Protocol: corev1.ProtocolUDP}},
		},
	}

	candidate := current.DeepCopy()
	candidate.Spec.Ports[0].Port = 3479

	v := NewServiceLens(candidate)
	if v.EqualResource(current) {
		t.Fatalf("expected service lens difference to be detected")
	}
}

func TestServiceApply(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}}
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "svc",
			Namespace:   "default",
			Labels:      map[string]string{"app": "stunner"},
			Annotations: map[string]string{"a": "1"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Gateway",
				Name:       "gw",
			}},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": "stunner"},
			Ports: []corev1.ServicePort{{
				Name:     "udp",
				Port:     3478,
				Protocol: corev1.ProtocolUDP,
			}},
		},
	}

	v := NewServiceLens(desired)
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !v.EqualResource(current) {
		t.Fatalf("expected applied service to match desired owned lens")
	}
}

func TestServiceApplyPreservesExternalMetadata(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "svc",
		Namespace: "default",
		Labels: map[string]string{
			"external-label": "keep",
		},
		Annotations: map[string]string{
			"external-ann": "keep",
		},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "external-owner",
		}},
	}}

	desired := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "svc",
		Namespace: "default",
		Labels: map[string]string{
			"owned-label": "set",
		},
		Annotations: map[string]string{
			"owned-ann": "set",
		},
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: "v1",
			Kind:       "Gateway",
			Name:       "owned-owner",
		}},
	}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}

	v := NewServiceLens(desired)
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if current.Labels["external-label"] != "keep" || current.Labels["owned-label"] != "set" {
		t.Fatalf("service labels should retain external and add owned labels")
	}

	if current.Annotations["external-ann"] != "keep" || current.Annotations["owned-ann"] != "set" {
		t.Fatalf("service annotations should retain external and add owned annotations")
	}

	if len(current.OwnerReferences) != 2 {
		t.Fatalf("service ownerrefs should keep external and add owned ownerref")
	}
}

func ptrBool(v bool) *bool {
	return &v
}
