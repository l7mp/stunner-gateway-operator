package updater

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/lens"
)

func TestServiceNoopSuppress(t *testing.T) {
	current := testService()
	svcLens := lens.NewServiceLens(current.DeepCopy())
	assert.True(t, svcLens.EqualResource(current), "expected service no-op to be suppressed")

	desired := current.DeepCopy()
	desired.Spec.Ports[0].Port = 3479
	svcLens = lens.NewServiceLens(desired)
	assert.False(t, svcLens.EqualResource(current), "expected changed service not to be suppressed")
}

func TestServiceMutate(t *testing.T) {
	current := testService()
	current.Labels["preserve"] = "yes"
	current.Annotations["legacy"] = "keep"
	lbClass := "service.k8s.aws/nlb"
	current.Spec.LoadBalancerClass = &lbClass

	desired := testService()
	desired.Labels["new"] = "label"
	desired.Annotations["new"] = "annotation"
	desired.Spec.Type = corev1.ServiceTypeNodePort
	desired.Spec.Ports[0].Port = 3479

	v := lens.NewServiceLens(desired)
	require.NoError(t, v.ApplyToResource(current), "unexpected mutate error")

	assert.Equal(t, corev1.ServiceTypeNodePort, current.Spec.Type, "service type should be copied")
	assert.Equal(t, int32(3479), current.Spec.Ports[0].Port, "service port should be copied")
	assert.Equal(t, "label", current.Labels["new"], "service labels should include new labels")
	assert.Equal(t, "yes", current.Labels["preserve"], "service labels should preserve existing labels")
	assert.Equal(t, "annotation", current.Annotations["new"],
		"service annotations should include new annotations")
	assert.Equal(t, "keep", current.Annotations["legacy"],
		"service annotations should preserve existing annotations")
	require.Len(t, current.OwnerReferences, 1, "service ownerRef should be set")
	assert.Equal(t, desired.OwnerReferences[0].Name, current.OwnerReferences[0].Name,
		"service ownerRef should be set")
	require.NotNil(t, current.Spec.LoadBalancerClass,
		"externally managed loadBalancerClass should be preserved")
	assert.Equal(t, "service.k8s.aws/nlb", *current.Spec.LoadBalancerClass,
		"externally managed loadBalancerClass should be preserved")
}

func TestDeploymentMutate(t *testing.T) {
	current := testDeployment()
	desired := testDeployment()

	replicas := int32(3)
	desired.Spec.Replicas = &replicas
	desired.Spec.Template.Spec.HostNetwork = true
	desired.Spec.Template.Spec.Containers[0].Image = "stunnerd:v2"
	desired.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "regcred"}}
	desired.Spec.Template.Spec.TerminationGracePeriodSeconds = ptrInt64(12)
	desired.Spec.Template.Spec.Tolerations = []corev1.Toleration{{Key: "dedicated", Operator: corev1.TolerationOpEqual, Value: "edge"}}
	desired.Spec.Template.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{MaxSkew: 1, TopologyKey: "zone", WhenUnsatisfiable: corev1.ScheduleAnyway}}

	v := lens.NewDeploymentLens(desired)
	require.NoError(t, v.ApplyToResource(current), "unexpected mutate error")

	assert.True(t,
		apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers),
		"containers not copied")
	assert.True(t,
		apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.ImagePullSecrets, desired.Spec.Template.Spec.ImagePullSecrets),
		"image pull secrets not copied")
	assert.True(t,
		apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.Tolerations, desired.Spec.Template.Spec.Tolerations),
		"tolerations not copied")
	assert.True(t,
		apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.TopologySpreadConstraints,
			desired.Spec.Template.Spec.TopologySpreadConstraints),
		"topology spread constraints not copied")
	require.NotNil(t, current.Spec.Template.Spec.TerminationGracePeriodSeconds,
		"termination grace period should be copied")
	assert.Equal(t, int64(12), *current.Spec.Template.Spec.TerminationGracePeriodSeconds,
		"termination grace period not copied")
	assert.True(t, current.Spec.Template.Spec.HostNetwork, "hostNetwork not copied")
	require.NotNil(t, current.Spec.Replicas, "replicas should be enforced")
	assert.Equal(t, int32(3), *current.Spec.Replicas, "replicas not enforced")
}

func TestDeploymentReplicaPreserve(t *testing.T) {
	current := testDeployment()
	currentReplicas := int32(5)
	current.Spec.Replicas = &currentReplicas

	desired := testDeployment()
	one := int32(1)
	desired.Spec.Replicas = &one

	v := lens.NewDeploymentLens(desired)
	require.NoError(t, v.ApplyToResource(current), "unexpected mutate error")

	require.NotNil(t, current.Spec.Replicas, "replicas should be preserved")
	assert.Equal(t, int32(5), *current.Spec.Replicas,
		"single replica should not overwrite existing replica count")
}

func TestDeploymentNoopSuppress(t *testing.T) {
	current := testDeployment()
	v := lens.NewDeploymentLens(current.DeepCopy())
	assert.True(t, v.EqualResource(current), "expected deployment no-op to be suppressed")

	desired := current.DeepCopy()
	desired.Spec.Template.Spec.Containers[0].Image = "stunnerd:v3"
	v = lens.NewDeploymentLens(desired)
	assert.False(t, v.EqualResource(current), "expected changed deployment not to be suppressed")
}

func TestGatewayClassStatusEqual(t *testing.T) {
	now := metav1.NewTime(time.Now())
	later := metav1.NewTime(time.Now().Add(10 * time.Second))

	current := gwapiv1.GatewayClassStatus{Conditions: []metav1.Condition{{
		Type:               string(gwapiv1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gwapiv1.GatewayClassReasonAccepted),
		Message:            "ok",
		ObservedGeneration: 1,
		LastTransitionTime: now,
	}}}
	desired := current.DeepCopy()
	desired.Conditions[0].LastTransitionTime = later

	assert.True(t, lens.GatewayClassStatusEqual(current, desired),
		"expected equal statuses to ignore LastTransitionTime differences")

	desired.Conditions[0].Reason = "Different"
	assert.False(t, lens.GatewayClassStatusEqual(current, desired),
		"expected semantic difference to be detected")
}

func TestGatewayStatusEqual(t *testing.T) {
	now := metav1.NewTime(time.Now())
	later := metav1.NewTime(time.Now().Add(10 * time.Second))

	current := gwapiv1.GatewayStatus{
		Conditions: []metav1.Condition{{
			Type:               string(gwapiv1.GatewayConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwapiv1.GatewayReasonAccepted),
			ObservedGeneration: 3,
			LastTransitionTime: now,
		}},
		Listeners: []gwapiv1.ListenerStatus{{
			Name: gwapiv1.SectionName("udp"),
			Conditions: []metav1.Condition{{
				Type:               string(gwapiv1.ListenerConditionAccepted),
				Status:             metav1.ConditionTrue,
				Reason:             string(gwapiv1.ListenerReasonAccepted),
				ObservedGeneration: 3,
				LastTransitionTime: now,
			}},
		}},
	}

	desired := current.DeepCopy()
	desired.Conditions[0].LastTransitionTime = later
	desired.Listeners[0].Conditions[0].LastTransitionTime = later

	assert.True(t, lens.GatewayStatusEqual(current, desired),
		"expected equal gateway status to ignore timestamp differences")

	desired.Listeners[0].Conditions[0].Reason = "Different"
	assert.False(t, lens.GatewayStatusEqual(current, desired),
		"expected listener semantic difference to be detected")
}

func TestGatewayStatusAddressDiff(t *testing.T) {
	current := gwapiv1.GatewayStatus{
		Addresses: []gwapiv1.GatewayStatusAddress{{
			Type:  ptrAddressType(gwapiv1.IPAddressType),
			Value: "203.0.113.10",
		}},
	}

	desired := current.DeepCopy()
	desired.Addresses[0].Value = "203.0.113.11"

	assert.False(t, lens.GatewayStatusEqual(current, desired), "expected address change to be detected")
}

func TestUDPRouteStatusEqual(t *testing.T) {
	now := metav1.NewTime(time.Now())
	later := metav1.NewTime(time.Now().Add(10 * time.Second))

	group := gwapiv1.Group(gwapiv1.GroupName)
	kind := gwapiv1.Kind("Gateway")
	name := gwapiv1.ObjectName("gw")

	current := gwapiv1a2.UDPRouteStatus{RouteStatus: gwapiv1a2.RouteStatus{Parents: []gwapiv1.RouteParentStatus{{
		ParentRef:      gwapiv1.ParentReference{Group: &group, Kind: &kind, Name: name},
		ControllerName: gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
		Conditions: []metav1.Condition{{
			Type:               string(gwapiv1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             string(gwapiv1.RouteReasonAccepted),
			ObservedGeneration: 2,
			LastTransitionTime: now,
		}},
	}}}}

	desired := *current.DeepCopy()
	desired.Parents[0].Conditions[0].LastTransitionTime = later

	assert.True(t, lens.UDPRouteStatusEqual(current, desired),
		"expected equal UDPRoute status to ignore timestamp differences")

	desired.Parents[0].Conditions[0].Message = "changed"
	assert.False(t, lens.UDPRouteStatusEqual(current, desired),
		"expected parent condition semantic difference to be detected")
}

func TestUDPRouteStatusParentDiff(t *testing.T) {
	group := gwapiv1.Group(gwapiv1.GroupName)
	kind := gwapiv1.Kind("Gateway")
	name := gwapiv1.ObjectName("gw")

	current := gwapiv1a2.UDPRouteStatus{RouteStatus: gwapiv1a2.RouteStatus{Parents: []gwapiv1.RouteParentStatus{{
		ParentRef:      gwapiv1.ParentReference{Group: &group, Kind: &kind, Name: name},
		ControllerName: gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
	}}}}

	desired := *current.DeepCopy()
	desired.Parents[0].ParentRef.Name = gwapiv1.ObjectName("gw-other")

	assert.False(t, lens.UDPRouteStatusEqual(current, desired),
		"expected parent ref change to be detected")
}

func testService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "svc",
			Namespace:   "default",
			Labels:      map[string]string{"app": "stunner"},
			Annotations: map[string]string{"team": "edge"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Gateway",
				Name:       "gw",
				UID:        "uid-1",
			}},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": "stunner"},
			Ports: []corev1.ServicePort{{
				Name:     "udp",
				Protocol: corev1.ProtocolUDP,
				Port:     3478,
			}},
		},
	}
}

func testDeployment() *appv1.Deployment {
	replicas := int32(2)
	return &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp",
			Namespace: "default",
			Labels:    map[string]string{"app": "stunner"},
			Annotations: map[string]string{
				"team": "edge",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Gateway",
				Name:       "gw",
				UID:        "uid-1",
			}},
		},
		Spec: appv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "stunner"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": "stunner"},
					Annotations: map[string]string{"ann": "1"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "stunnerd",
						Image: "stunnerd:v1",
					}},
					Volumes: []corev1.Volume{{Name: "v1"}},
				},
			},
		},
	}
}

func ptrInt64(v int64) *int64 {
	return &v
}

func ptrAddressType(v gwapiv1.AddressType) *gwapiv1.AddressType {
	return &v
}
