package renderer

import (
	"fmt"
	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
)

// maxConds is the maximum number of conditions that can be stored at one in a Gateway object
const maxGwayStatusConds = 8

type listenerRoutePair struct {
	listener *gatewayv1alpha2.Listener
	route    *gatewayv1alpha2.UDPRoute
}

func (r *Renderer) getGateways4Class(gc *gatewayv1alpha2.GatewayClass) []*gatewayv1alpha2.Gateway {
	gws := r.op.GetGateways()

	ret := make([]*gatewayv1alpha2.Gateway, 0)
	for _, g := range gws {
		if string(g.Spec.GatewayClassName) == gc.GetName() {
			ret = append(ret, g)
		}
	}

	return ret
}

func isGatewayScheduled(gw *gatewayv1alpha2.Gateway) bool {
	for _, cond := range gw.Status.Conditions {
		if cond.Type == string(gatewayv1alpha2.GatewayConditionScheduled) &&
			cond.Reason == string(gatewayv1alpha2.GatewayReasonScheduled) &&
			cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func isGatewayReady(gw *gatewayv1alpha2.Gateway) bool {
	for _, cond := range gw.Status.Conditions {
		if cond.Type == string(gatewayv1alpha2.GatewayConditionReady) &&
			cond.Reason == string(gatewayv1alpha2.GatewayReasonReady) &&
			cond.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func pruneGatewayStatusConds(gw *gatewayv1alpha2.Gateway) *gatewayv1alpha2.Gateway {
	if len(gw.Status.Conditions) >= maxGwayStatusConds {
		gw.Status.Conditions =
			gw.Status.Conditions[len(gw.Status.Conditions)-(maxGwayStatusConds-1):]
	}

	for _, s := range gw.Status.Listeners {
		if len(s.Conditions) >= maxGwayStatusConds {
			s.Conditions =
				s.Conditions[len(s.Conditions)-(maxGwayStatusConds-1):]
		}
	}

	return gw
}

// gateway status
func setGatewayStatusScheduled(gw *gatewayv1alpha2.Gateway, cname string) {
	gw.Status.Conditions = append(gw.Status.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.GatewayConditionScheduled),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.GatewayReasonScheduled),
		Message:            fmt.Sprintf("gateway under processing by controller %q", cname),
	})
}

func setGatewayStatusReady(gw *gatewayv1alpha2.Gateway, cname string) {
	gw.Status.Conditions = append(gw.Status.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.GatewayConditionReady),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.GatewayReasonReady),
		Message:            fmt.Sprintf("gateway processed by controller %q", cname),
	})
}

// listener status
func getStatus4Listener(gw *gatewayv1alpha2.Gateway, sectionName gatewayv1alpha2.SectionName) *gatewayv1alpha2.ListenerStatus {
	for _, s := range gw.Status.Listeners {
		if s.Name == sectionName {
			return &s
		}
	}
	return nil
}

func initListenerStatus(sectionName gatewayv1alpha2.SectionName) *gatewayv1alpha2.ListenerStatus {
	group := gatewayv1alpha2.Group(gatewayv1alpha2.GroupVersion.Group)
	return &gatewayv1alpha2.ListenerStatus{
		Name: sectionName,
		SupportedKinds: []gatewayv1alpha2.RouteGroupKind{{
			Group: &group,
			Kind:  gatewayv1alpha2.Kind("UDPRoute"),
		}},
	}
}

func setListenerStatusResolved(gw *gatewayv1alpha2.Gateway, s *gatewayv1alpha2.ListenerStatus, routes int) {
	s.Conditions = append(s.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.ListenerConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.ListenerReasonResolvedRefs),
		Message:            "listener processed",
	})
	s.AttachedRoutes = int32(routes)
}
