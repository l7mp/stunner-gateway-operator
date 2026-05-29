package lens

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
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

func TestServiceApplyPreservesExternallyManagedSpecFields(t *testing.T) {
	lbClass := "service.k8s.aws/nlb"
	ipFamilyPolicy := corev1.IPFamilyPolicySingleStack

	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "svc",
		Namespace: "default",
	}, Spec: corev1.ServiceSpec{
		Type:     corev1.ServiceTypeLoadBalancer,
		Selector: map[string]string{"app": "old"},
		Ports: []corev1.ServicePort{{
			Name:     "udp",
			Port:     3478,
			Protocol: corev1.ProtocolUDP,
		}},
		LoadBalancerClass: &lbClass,
		ClusterIP:         "10.0.0.10",
		ClusterIPs:        []string{"10.0.0.10"},
		IPFamilies:        []corev1.IPFamily{corev1.IPv4Protocol},
		IPFamilyPolicy:    &ipFamilyPolicy,
	}}

	desired := current.DeepCopy()
	desired.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1",
		Kind:       "Gateway",
		Name:       "gw",
	}}
	desired.Spec.Selector = map[string]string{"app": "stunner"}
	desired.Spec.Ports[0].Port = 3479
	desired.Spec.LoadBalancerClass = nil
	desired.Spec.ClusterIP = ""
	desired.Spec.ClusterIPs = nil
	desired.Spec.IPFamilies = nil
	desired.Spec.IPFamilyPolicy = nil

	v := NewServiceLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Equal(t, map[string]string{"app": "stunner"}, current.Spec.Selector,
		"owned selector should be copied")
	assert.Equal(t, int32(3479), current.Spec.Ports[0].Port,
		"owned service port should be copied")
	require.NotNil(t, current.Spec.LoadBalancerClass,
		"externally managed loadBalancerClass should be preserved")
	assert.Equal(t, "service.k8s.aws/nlb", *current.Spec.LoadBalancerClass,
		"externally managed loadBalancerClass should be preserved")
	assert.Equal(t, "10.0.0.10", current.Spec.ClusterIP,
		"clusterIP should be preserved")
	assert.Equal(t, []string{"10.0.0.10"}, current.Spec.ClusterIPs,
		"clusterIPs should be preserved")
	assert.Equal(t, []corev1.IPFamily{corev1.IPv4Protocol}, current.Spec.IPFamilies,
		"ipFamilies should be preserved")
	require.NotNil(t, current.Spec.IPFamilyPolicy,
		"ipFamilyPolicy should be preserved")
	assert.Equal(t, corev1.IPFamilyPolicySingleStack, *current.Spec.IPFamilyPolicy,
		"ipFamilyPolicy should be preserved")
}

func TestServiceApplyCopiesLoadBalancerIPWhenOwned(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}, Spec: corev1.ServiceSpec{
		Type:           corev1.ServiceTypeLoadBalancer,
		LoadBalancerIP: "203.0.113.10",
		Ports: []corev1.ServicePort{{
			Name:     "udp",
			Port:     3478,
			Protocol: corev1.ProtocolUDP,
		}},
	}}

	desired := current.DeepCopy()
	desired.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1",
		Kind:       "Gateway",
		Name:       "gw",
	}}
	desired.Spec.LoadBalancerIP = "198.51.100.20"

	v := NewServiceLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Equal(t, "198.51.100.20", current.Spec.LoadBalancerIP,
		"loadBalancerIP should be copied when explicitly owned")
}

func TestServiceEqualIgnoresLoadBalancerIPWhenNotOwned(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}, Spec: corev1.ServiceSpec{
		Type:           corev1.ServiceTypeLoadBalancer,
		LoadBalancerIP: "203.0.113.10",
		Ports: []corev1.ServicePort{{
			Name:     "udp",
			Port:     3478,
			Protocol: corev1.ProtocolUDP,
		}},
	}}

	desired := current.DeepCopy()
	desired.Spec.LoadBalancerIP = ""

	v := NewServiceLens(desired)
	assert.True(t, v.EqualResource(current),
		"loadBalancerIP should be ignored when desired does not own it")
}

func TestServiceEqualDetectsOwnedLoadBalancerIPDiff(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}, Spec: corev1.ServiceSpec{
		Type:           corev1.ServiceTypeLoadBalancer,
		LoadBalancerIP: "203.0.113.10",
		Ports: []corev1.ServicePort{{
			Name:     "udp",
			Port:     3478,
			Protocol: corev1.ProtocolUDP,
		}},
	}}

	desired := current.DeepCopy()
	desired.Spec.LoadBalancerIP = "198.51.100.20"

	v := NewServiceLens(desired)
	assert.False(t, v.EqualResource(current),
		"loadBalancerIP should be compared when desired owns it")
}

func TestServiceApplyPreservesNodePortWhenNotOwned(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeLoadBalancer,
		Ports: []corev1.ServicePort{{
			Name:     "udp",
			Port:     3478,
			Protocol: corev1.ProtocolUDP,
			NodePort: 32000,
		}},
	}}

	desired := current.DeepCopy()
	desired.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1",
		Kind:       "Gateway",
		Name:       "gw",
	}}
	desired.Spec.Ports[0].NodePort = 0

	v := NewServiceLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Equal(t, int32(32000), current.Spec.Ports[0].NodePort,
		"nodeport should be preserved when renderer does not own it")
}

func TestServiceApplyCopiesNodePortWhenOwned(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name:      "svc",
		Namespace: "default",
		Annotations: map[string]string{
			opdefault.NodePortAnnotationKey: `{"udp":32010}`,
		},
	}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeLoadBalancer,
		Ports: []corev1.ServicePort{{
			Name:     "udp",
			Port:     3478,
			Protocol: corev1.ProtocolUDP,
			NodePort: 32000,
		}},
	}}

	desired := current.DeepCopy()
	desired.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1",
		Kind:       "Gateway",
		Name:       "gw",
	}}
	desired.Spec.Ports[0].NodePort = 32010

	v := NewServiceLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Equal(t, int32(32010), current.Spec.Ports[0].NodePort,
		"nodeport should be copied when renderer owns it")
}

func TestServiceApplyPreservesNodePortForNodePortServiceWhenNotOwned(t *testing.T) {
	current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"}, Spec: corev1.ServiceSpec{
		Type: corev1.ServiceTypeNodePort,
		Ports: []corev1.ServicePort{{
			Name:     "udp",
			Port:     3478,
			Protocol: corev1.ProtocolUDP,
			NodePort: 32000,
		}},
	}}

	desired := current.DeepCopy()
	desired.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1",
		Kind:       "Gateway",
		Name:       "gw",
	}}
	desired.Spec.Ports[0].NodePort = 0

	v := NewServiceLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Equal(t, int32(32000), current.Spec.Ports[0].NodePort,
		"nodeport should be preserved when no nodeport annotation owns it")
}

func ptrBool(v bool) *bool {
	return &v
}
