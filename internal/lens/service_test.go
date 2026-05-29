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

func TestServiceLoadBalancerIPOwnership(t *testing.T) {
	cases := []struct {
		name        string
		currentIP   string
		desiredIP   string // empty signals "not owned by renderer"
		wantEqual   bool
		wantApplyIP string
	}{
		{
			name:        "not owned, current set",
			currentIP:   "203.0.113.10",
			desiredIP:   "",
			wantEqual:   true,
			wantApplyIP: "203.0.113.10",
		},
		{
			name:        "owned, equal",
			currentIP:   "203.0.113.10",
			desiredIP:   "203.0.113.10",
			wantEqual:   true,
			wantApplyIP: "203.0.113.10",
		},
		{
			name:        "owned, drift",
			currentIP:   "203.0.113.10",
			desiredIP:   "198.51.100.20",
			wantEqual:   false,
			wantApplyIP: "198.51.100.20",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := loadBalancerService()
			current.Spec.LoadBalancerIP = tc.currentIP

			desired := loadBalancerService()
			desired.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Gateway",
				Name:       "gw",
			}}
			current.OwnerReferences = desired.OwnerReferences
			desired.Spec.LoadBalancerIP = tc.desiredIP

			v := NewServiceLens(desired)
			assert.Equal(t, tc.wantEqual, v.EqualResource(current),
				"EqualResource result for LoadBalancerIP")

			require.NoError(t, v.ApplyToResource(current), "apply failed")
			assert.Equal(t, tc.wantApplyIP, current.Spec.LoadBalancerIP,
				"LoadBalancerIP after apply")
		})
	}
}

func TestServiceNodePortOwnership(t *testing.T) {
	cases := []struct {
		name              string
		serviceType       corev1.ServiceType
		hasNodePortAnno   bool
		currentNodePort   int32
		desiredNodePort   int32
		wantNodePortAfter int32
	}{
		{
			name:              "LoadBalancer, no annotation, preserve current",
			serviceType:       corev1.ServiceTypeLoadBalancer,
			currentNodePort:   32000,
			desiredNodePort:   0,
			wantNodePortAfter: 32000,
		},
		{
			name:              "LoadBalancer, with annotation, copy desired",
			serviceType:       corev1.ServiceTypeLoadBalancer,
			hasNodePortAnno:   true,
			currentNodePort:   32000,
			desiredNodePort:   32010,
			wantNodePortAfter: 32010,
		},
		{
			name:              "NodePort, no annotation, preserve current",
			serviceType:       corev1.ServiceTypeNodePort,
			currentNodePort:   32000,
			desiredNodePort:   0,
			wantNodePortAfter: 32000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name:      "svc",
				Namespace: "default",
			}, Spec: corev1.ServiceSpec{
				Type: tc.serviceType,
				Ports: []corev1.ServicePort{{
					Name:     "udp",
					Port:     3478,
					Protocol: corev1.ProtocolUDP,
					NodePort: tc.currentNodePort,
				}},
			}}

			if tc.hasNodePortAnno {
				current.Annotations = map[string]string{
					opdefault.NodePortAnnotationKey: `{"udp":32010}`,
				}
			}

			desired := current.DeepCopy()
			desired.OwnerReferences = []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Gateway",
				Name:       "gw",
			}}
			desired.Spec.Ports[0].NodePort = tc.desiredNodePort

			v := NewServiceLens(desired)
			require.NoError(t, v.ApplyToResource(current), "apply failed")

			assert.Equal(t, tc.wantNodePortAfter, current.Spec.Ports[0].NodePort,
				"NodePort after apply")
		})
	}
}

func loadBalancerService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{{
				Name:     "udp",
				Port:     3478,
				Protocol: corev1.ProtocolUDP,
			}},
		},
	}
}

func ptrBool(v bool) *bool {
	return &v
}
