package renderer

import (
	"fmt"
	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
)

// maxConds is the maximum number of conditions that can be stored at one in a Gateway object
const maxGwayStatusConds = 8

type listenerRoutePair struct {
	listener *gatewayv1alpha2.Listener
	route    *gatewayv1alpha2.UDPRoute
}

func (r *Renderer) getGateways4Class(gc *gatewayv1alpha2.GatewayClass) []*gatewayv1alpha2.Gateway {
	r.log.V(4).Info("getGateways4Class", "GatewayClass", store.GetObjectKey(gc))
	gws := r.op.GetGateways()

	ret := make([]*gatewayv1alpha2.Gateway, 0)
	for _, g := range gws {
		if string(g.Spec.GatewayClassName) == gc.GetName() {
			ret = append(ret, g)
		}
	}

	r.log.V(4).Info("getGateways4Class: ready", "GatewayClass", store.GetObjectKey(gc),
		"gateways", len(ret))

	return ret
}

func pruneGatewayStatusConds(gw *gatewayv1alpha2.Gateway) *gatewayv1alpha2.Gateway {
	if len(gw.Status.Conditions) >= maxGwayStatusConds {
		gw.Status.Conditions =
			gw.Status.Conditions[len(gw.Status.Conditions)-(maxGwayStatusConds-1):]
	}

	return gw
}

// gateway status
func setGatewayStatusScheduled(gw *gatewayv1alpha2.Gateway, cname string) {
	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.GatewayConditionScheduled),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.GatewayReasonScheduled),
		Message:            fmt.Sprintf("gateway under processing by controller %q", cname),
	})

	// reinit listener statuses
	gw.Status.Listeners = gw.Status.Listeners[:0]
	group := gatewayv1alpha2.Group(gatewayv1alpha2.GroupVersion.Group)

	for _, l := range gw.Spec.Listeners {
		gw.Status.Listeners = append(gw.Status.Listeners,
			gatewayv1alpha2.ListenerStatus{
				Name: l.Name,
				SupportedKinds: []gatewayv1alpha2.RouteGroupKind{{
					Group: &group,
					Kind:  gatewayv1alpha2.Kind("UDPRoute"),
				}},
				Conditions: []metav1.Condition{},
			})
	}

}

func setGatewayStatusReady(gw *gatewayv1alpha2.Gateway, cname string) {
	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.GatewayConditionReady),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.GatewayReasonReady),
		Message:            fmt.Sprintf("gateway processed by controller %q", cname),
	})
}

// listener status
func getStatus4Listener(gw *gatewayv1alpha2.Gateway, l *gatewayv1alpha2.Listener) *gatewayv1alpha2.ListenerStatus {
	for i := range gw.Status.Listeners {
		if gw.Status.Listeners[i].Name == l.Name {
			return &gw.Status.Listeners[i]
		}
	}
	return nil
}

// sets "Detached" to true with reason "UnsupportedProtocol" or false, depending on "accepted"
// sets ResolvedRefs to true
// sets "Ready" to <ready> depending on "ready"
func setListenerStatus(gw *gatewayv1alpha2.Gateway, l *gatewayv1alpha2.Listener, accepted bool, ready bool, routes int) {
	s := getStatus4Listener(gw, l)
	if s == nil {
		// should never happen
		return
	}

	setListenerStatusDetached(gw, s, accepted)
	setListenerStatusResolvedRefs(gw, s)
	setListenerStatusReady(gw, s, ready)
	s.AttachedRoutes = int32(routes)
}

func setListenerStatusDetached(gw *gatewayv1alpha2.Gateway, s *gatewayv1alpha2.ListenerStatus, accepted bool) {
	if accepted == true {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gatewayv1alpha2.ListenerConditionDetached),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gatewayv1alpha2.ListenerReasonAttached),
			Message:            "listener accepted",
		})
	} else {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gatewayv1alpha2.ListenerConditionDetached),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gatewayv1alpha2.ListenerReasonUnsupportedProtocol),
			Message:            "unsupported protocol",
		})
	}
}

func setListenerStatusResolvedRefs(gw *gatewayv1alpha2.Gateway, s *gatewayv1alpha2.ListenerStatus) {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.ListenerConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.ListenerReasonResolvedRefs),
		Message:            "listener object references sucessfully resolved",
	})
}

func setListenerStatusReady(gw *gatewayv1alpha2.Gateway, s *gatewayv1alpha2.ListenerStatus, ready bool) {
	if ready == true {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gatewayv1alpha2.ListenerConditionReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gatewayv1alpha2.ListenerReasonReady),
			Message:            "public address found for gateway",
		})
	} else {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gatewayv1alpha2.ListenerConditionReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gatewayv1alpha2.ListenerReasonPending),
			Message:            "public address pending",
		})
	}
}
