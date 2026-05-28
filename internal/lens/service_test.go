package lens

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.True(t, v.EqualResource(current),
		"expected service lenses to match after default normalization")
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
	assert.False(t, v.EqualResource(current), "expected service lens difference to be detected")
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
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.True(t, v.EqualResource(current), "expected applied service to match desired owned lens")
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
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Equal(t, "keep", current.Labels["external-label"],
		"service labels should retain external labels")
	assert.Equal(t, "set", current.Labels["owned-label"],
		"service labels should add owned labels")

	assert.Equal(t, "keep", current.Annotations["external-ann"],
		"service annotations should retain external annotations")
	assert.Equal(t, "set", current.Annotations["owned-ann"],
		"service annotations should add owned annotations")

	assert.Len(t, current.OwnerReferences, 2,
		"service ownerrefs should keep external and add owned ownerref")
}

func ptrBool(v bool) *bool {
	return &v
}
