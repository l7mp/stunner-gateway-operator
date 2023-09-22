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

	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// maxConds is the maximum number of conditions that can be stored at one in a Gateway object
const maxGwayStatusConds = 8

type portMap map[int]bool

func (r *Renderer) getGateways4Class(c *RenderContext) []*gwapiv1b1.Gateway {
	// r.log.V(4).Info("getGateways4Class", "gateway-class", store.GetObjectKey(c.gc))

	ret := []*gwapiv1b1.Gateway{}

	for _, g := range store.Gateways.GetAll() {
		if string(g.Spec.GatewayClassName) == c.gc.GetName() {
			ret = append(ret, g)
		}
	}

	r.log.V(4).Info("getGateways4Class: ready", "gateway-class", store.GetObjectKey(c.gc),
		"gateways", len(ret))

	return ret
}

func pruneGatewayStatusConds(gw *gwapiv1b1.Gateway) *gwapiv1b1.Gateway {
	if len(gw.Status.Conditions) >= maxGwayStatusConds {
		gw.Status.Conditions =
			gw.Status.Conditions[len(gw.Status.Conditions)-(maxGwayStatusConds-1):]
	}

	return gw
}

func isListenerConflicted(l *gwapiv1b1.Listener, udpPorts portMap, tcpPorts portMap) bool {
	switch l.Protocol {
	case "UDP", "DTLS":
		_, ok := udpPorts[int(l.Port)]
		udpPorts[int(l.Port)] = true
		return ok
	case "TCP", "TLS":
		_, ok := tcpPorts[int(l.Port)]
		tcpPorts[int(l.Port)] = true
		return ok
	}
	// unknown protocol
	return false
}

// gateway status
func initGatewayStatus(gw *gwapiv1b1.Gateway, cname string) {
	gw.Status.Addresses = []gwapiv1b1.GatewayStatusAddress{}

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
	group := gwapiv1b1.Group(gwapiv1b1.GroupVersion.Group)

	for _, l := range gw.Spec.Listeners {
		gw.Status.Listeners = append(gw.Status.Listeners,
			gwapiv1b1.ListenerStatus{
				Name: l.Name,
				SupportedKinds: []gwapiv1b1.RouteGroupKind{{
					Group: &group,
					Kind:  gwapiv1b1.Kind("UDPRoute"),
				}},
				Conditions: []metav1.Condition{},
			})
	}
}

func setGatewayStatusProgrammed(gw *gwapiv1b1.Gateway, err error, ap *gatewayAddress) {
	if err != nil {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.GatewayConditionProgrammed),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.GatewayReasonInvalid),
			Message: fmt.Sprintf("error processing gateway by controller %q: %s",
				config.ControllerName, err.Error()),
		})
		return
	}

	if ap != nil {
		aType := ap.aType
		if string(aType) == "" {
			aType = gwapiv1b1.IPAddressType
		}
		gw.Status.Addresses = []gwapiv1b1.GatewayStatusAddress{{
			Type:  &aType,
			Value: ap.addr,
		}}
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.GatewayConditionProgrammed),
			Message:            "dataplane configuration successfully rendered",
		})
	} else {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.GatewayConditionProgrammed),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.GatewayReasonAddressNotAssigned),
			Message:            "no public address found",
		})
	}
}

// listener status
func getStatus4Listener(gw *gwapiv1b1.Gateway, l *gwapiv1b1.Listener) *gwapiv1b1.ListenerStatus {
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
func setListenerStatus(gw *gwapiv1b1.Gateway, l *gwapiv1b1.Listener, err error, conflicted bool, routes int) {
	s := getStatus4Listener(gw, l)
	if s == nil {
		// should never happen
		return
	}

	setListenerStatusAccepted(gw, s, err)
	setListenerStatusConflicted(gw, s, conflicted)
	setListenerStatusResolvedRefs(gw, s)
	// listener ready status deprecated
	// setListenerStatusReady(gw, s, ready)
	s.AttachedRoutes = int32(routes)
}

func setListenerStatusAccepted(gw *gwapiv1b1.Gateway, s *gwapiv1b1.ListenerStatus, reason error) {
	switch {
	case IsNonCriticalError(reason, PortUnavailable):
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonPortUnavailable),
			Message:            "port unavailable",
		})
	case IsNonCriticalError(reason, InvalidProtocol):
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonUnsupportedProtocol),
			Message:            "unsupported protocol",
		})
	case reason != nil:
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionAccepted),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonPending),
			Message:            "pending",
		})
	default:
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonAccepted),
			Message:            "listener accepted",
		})
	}
}

func setListenerStatusConflicted(gw *gwapiv1b1.Gateway, s *gwapiv1b1.ListenerStatus, conflicted bool) {
	if !conflicted {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionConflicted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonNoConflicts),
			Message:            "listener protocol-port available",
		})
	} else {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.ListenerConditionConflicted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.ListenerReasonProtocolConflict),
			Message:            "multiple listeners specified with the same Listener port and protocol",
		})
	}
}

func setListenerStatusResolvedRefs(gw *gwapiv1b1.Gateway, s *gwapiv1b1.ListenerStatus) {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(gwapiv1b1.ListenerConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1b1.ListenerReasonResolvedRefs),
		Message:            "listener object references sucessfully resolved",
	})
}

// func setListenerStatusReady(gw *gwapiv1b1.Gateway, s *gwapiv1b1.ListenerStatus, ready bool) {
// 	if ready {
// 		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
// 			Type:               string(gwapiv1b1.ListenerConditionReady),
// 			Status:             metav1.ConditionTrue,
// 			ObservedGeneration: gw.Generation,
// 			LastTransitionTime: metav1.Now(),
// 			Reason:             string(gwapiv1b1.ListenerReasonReady),
// 			Message:            "public address found for gateway",
// 		})
// 	} else {
// 		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
// 			Type:               string(gwapiv1b1.ListenerConditionReady),
// 			Status:             metav1.ConditionFalse,
// 			ObservedGeneration: gw.Generation,
// 			LastTransitionTime: metav1.Now(),
// 			Reason:             string(gwapiv1b1.ListenerReasonPending),
// 			Message:            "public address pending",
// 		})
// 	}
// }
