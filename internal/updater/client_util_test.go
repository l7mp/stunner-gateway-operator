package updater

import (
	"testing"
	"time"

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
	if !svcLens.EqualResource(current) {
		t.Fatalf("expected service no-op to be suppressed")
	}

	desired := current.DeepCopy()
	desired.Spec.Ports[0].Port = 3479
	svcLens = lens.NewServiceLens(desired)
	if svcLens.EqualResource(current) {
		t.Fatalf("expected changed service not to be suppressed")
	}
}

func TestServiceMutate(t *testing.T) {
	current := testService()
	current.Labels["preserve"] = "yes"
	current.Annotations["legacy"] = "keep"

	desired := testService()
	desired.Labels["new"] = "label"
	desired.Annotations["new"] = "annotation"
	desired.Spec.Type = corev1.ServiceTypeNodePort
	desired.Spec.Ports[0].Port = 3479

	v := lens.NewServiceLens(desired)
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("unexpected mutate error: %v", err)
	}

	if current.Spec.Type != corev1.ServiceTypeNodePort || current.Spec.Ports[0].Port != 3479 {
		t.Fatalf("service spec not copied")
	}
	if current.Labels["new"] != "label" || current.Labels["preserve"] != "yes" {
		t.Fatalf("service labels were not merged as expected")
	}
	if current.Annotations["new"] != "annotation" || current.Annotations["legacy"] != "keep" {
		t.Fatalf("service annotations were not merged as expected")
	}
	if len(current.OwnerReferences) != 1 || current.OwnerReferences[0].Name != desired.OwnerReferences[0].Name {
		t.Fatalf("service ownerRef not set")
	}
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
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("unexpected mutate error: %v", err)
	}

	if !apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.Containers, desired.Spec.Template.Spec.Containers) {
		t.Fatalf("containers not copied")
	}
	if !apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.ImagePullSecrets, desired.Spec.Template.Spec.ImagePullSecrets) {
		t.Fatalf("image pull secrets not copied")
	}
	if !apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.Tolerations, desired.Spec.Template.Spec.Tolerations) {
		t.Fatalf("tolerations not copied")
	}
	if !apiequality.Semantic.DeepEqual(current.Spec.Template.Spec.TopologySpreadConstraints, desired.Spec.Template.Spec.TopologySpreadConstraints) {
		t.Fatalf("topology spread constraints not copied")
	}
	if current.Spec.Template.Spec.TerminationGracePeriodSeconds == nil || *current.Spec.Template.Spec.TerminationGracePeriodSeconds != 12 {
		t.Fatalf("termination grace period not copied")
	}
	if !current.Spec.Template.Spec.HostNetwork {
		t.Fatalf("hostNetwork not copied")
	}
	if current.Spec.Replicas == nil || *current.Spec.Replicas != 3 {
		t.Fatalf("replicas not enforced")
	}
}

func TestDeploymentReplicaPreserve(t *testing.T) {
	current := testDeployment()
	currentReplicas := int32(5)
	current.Spec.Replicas = &currentReplicas

	desired := testDeployment()
	one := int32(1)
	desired.Spec.Replicas = &one

	v := lens.NewDeploymentLens(desired)
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("unexpected mutate error: %v", err)
	}

	if current.Spec.Replicas == nil || *current.Spec.Replicas != 5 {
		t.Fatalf("single replica should not overwrite existing replica count")
	}
}

func TestDeploymentNoopSuppress(t *testing.T) {
	current := testDeployment()
	v := lens.NewDeploymentLens(current.DeepCopy())
	if !v.EqualResource(current) {
		t.Fatalf("expected deployment no-op to be suppressed")
	}

	desired := current.DeepCopy()
	desired.Spec.Template.Spec.Containers[0].Image = "stunnerd:v3"
	v = lens.NewDeploymentLens(desired)
	if v.EqualResource(current) {
		t.Fatalf("expected changed deployment not to be suppressed")
	}
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

	if !lens.GatewayClassStatusEqual(current, desired) {
		t.Fatalf("expected equal statuses to ignore LastTransitionTime differences")
	}

	desired.Conditions[0].Reason = "Different"
	if lens.GatewayClassStatusEqual(current, desired) {
		t.Fatalf("expected semantic difference to be detected")
	}
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

	if !lens.GatewayStatusEqual(current, desired) {
		t.Fatalf("expected equal gateway status to ignore timestamp differences")
	}

	desired.Listeners[0].Conditions[0].Reason = "Different"
	if lens.GatewayStatusEqual(current, desired) {
		t.Fatalf("expected listener semantic difference to be detected")
	}
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

	if lens.GatewayStatusEqual(current, desired) {
		t.Fatalf("expected address change to be detected")
	}
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

	if !lens.UDPRouteStatusEqual(current, desired) {
		t.Fatalf("expected equal UDPRoute status to ignore timestamp differences")
	}

	desired.Parents[0].Conditions[0].Message = "changed"
	if lens.UDPRouteStatusEqual(current, desired) {
		t.Fatalf("expected parent condition semantic difference to be detected")
	}
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

	if lens.UDPRouteStatusEqual(current, desired) {
		t.Fatalf("expected parent ref change to be detected")
	}
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
