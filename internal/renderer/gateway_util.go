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

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// maxConds is the maximum number of conditions that can be stored at one in a Gateway object
const maxGwayStatusConds = 8

func (r *Renderer) getGateways4Class(c *RenderContext) []*gwapiv1a2.Gateway {
	r.log.V(4).Info("getGateways4Class", "gateway-class", store.GetObjectKey(c.gc))

	ret := []*gwapiv1a2.Gateway{}

	for _, g := range store.Gateways.GetAll() {
		if string(g.Spec.GatewayClassName) == c.gc.GetName() {
			ret = append(ret, g)
		}
	}

	r.log.V(4).Info("getGateways4Class: ready", "gateway-class", store.GetObjectKey(c.gc),
		"gateways", len(ret))

	return ret
}

func pruneGatewayStatusConds(gw *gwapiv1a2.Gateway) *gwapiv1a2.Gateway {
	if len(gw.Status.Conditions) >= maxGwayStatusConds {
		gw.Status.Conditions =
			gw.Status.Conditions[len(gw.Status.Conditions)-(maxGwayStatusConds-1):]
	}

	return gw
}

// gateway status
func initGatewayStatus(gw *gwapiv1a2.Gateway, cname string) {
	// set accepted to true and programmed to pending
	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               string(gwapiv1b1.GatewayConditionAccepted),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1b1.GatewayReasonAccepted),
		Message: fmt.Sprintf("gateway accepted by controller %s",
			config.ControllerName),
	})

	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               string(gwapiv1b1.GatewayConditionProgrammed),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1b1.GatewayReasonPending),
		Message:            "gateway under processing",
	})

	// reinit listener statuses
	gw.Status.Listeners = gw.Status.Listeners[:0]
	group := gwapiv1a2.Group(gwapiv1a2.GroupVersion.Group)

	for _, l := range gw.Spec.Listeners {
		gw.Status.Listeners = append(gw.Status.Listeners,
			gwapiv1a2.ListenerStatus{
				Name: l.Name,
				SupportedKinds: []gwapiv1a2.RouteGroupKind{{
					Group: &group,
					Kind:  gwapiv1a2.Kind("UDPRoute"),
				}},
				Conditions: []metav1.Condition{},
			})
	}

}

func setGatewayStatusProgrammed(gw *gwapiv1a2.Gateway, err error) {
	if err == nil {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.GatewayConditionProgrammed),
			Message:            "dataplane configuration successfully rendered for gateway",
		})
	} else {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.GatewayConditionProgrammed),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.GatewayReasonInvalid),
			Message: fmt.Sprintf("error processing gateway by controller %q: %s",
				config.ControllerName, err.Error()),
		})
	}
}

// listener status
func getStatus4Listener(gw *gwapiv1a2.Gateway, l *gwapiv1a2.Listener) *gwapiv1a2.ListenerStatus {
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
func setListenerStatus(gw *gwapiv1a2.Gateway, l *gwapiv1a2.Listener, accepted bool, ready bool, routes int) {
	s := getStatus4Listener(gw, l)
	if s == nil {
		// should never happen
		return
	}

	setListenerStatusAccepted(gw, s, accepted)
	setListenerStatusResolvedRefs(gw, s)
	setListenerStatusReady(gw, s, ready)
	s.AttachedRoutes = int32(routes)
}

func setListenerStatusAccepted(gw *gwapiv1a2.Gateway, s *gwapiv1a2.ListenerStatus, accepted bool) {
	if accepted {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonAccepted),
			Message:            "listener accepted",
		})
	} else {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonUnsupportedProtocol),
			Message:            "unsupported protocol",
		})
	}
}

func setListenerStatusResolvedRefs(gw *gwapiv1a2.Gateway, s *gwapiv1a2.ListenerStatus) {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(gwapiv1b1.ListenerConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1b1.ListenerReasonResolvedRefs),
		Message:            "listener object references sucessfully resolved",
	})
}

func setListenerStatusReady(gw *gwapiv1a2.Gateway, s *gwapiv1a2.ListenerStatus, ready bool) {
	if ready {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionReady),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonReady),
			Message:            "public address found for gateway",
		})
	} else {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonPending),
			Message:            "public address pending",
		})
	}
}
