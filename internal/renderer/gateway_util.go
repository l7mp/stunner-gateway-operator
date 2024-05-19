package renderer

import (
	"fmt"
	"strings"

	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// maxConds is the maximum number of conditions that can be stored at one in a Gateway object
const maxGwayStatusConds = 8

type portMap map[int]bool

func (r *Renderer) getGateways4Class(c *RenderContext) []*gwapiv1.Gateway {
	// r.log.V(4).Info("getGateways4Class", "gateway-class", store.GetObjectKey(c.gc))

	ret := []*gwapiv1.Gateway{}

	for _, g := range store.Gateways.GetAll() {
		if string(g.Spec.GatewayClassName) == c.gc.GetName() {
			ret = append(ret, g)
		}
	}

	r.log.V(4).Info("getGateways4Class: ready", "gateway-class", store.GetObjectKey(c.gc),
		"gateways", len(ret))

	return ret
}

func isManagedDataplaneDisabled(gw *gwapiv1.Gateway) bool {
	v, ok := gw.GetAnnotations()[opdefault.ManagedDataplaneDisabledAnnotationKey]
	return ok && strings.ToLower(v) == opdefault.ManagedDataplaneDisabledAnnotationValue
}

func pruneGatewayStatusConds(gw *gwapiv1.Gateway) *gwapiv1.Gateway {
	if len(gw.Status.Conditions) >= maxGwayStatusConds {
		gw.Status.Conditions =
			gw.Status.Conditions[len(gw.Status.Conditions)-(maxGwayStatusConds-1):]
	}

	return gw
}

func isListenerConflicted(l *gwapiv1.Listener, udpPorts portMap, tcpPorts portMap) bool {
	switch l.Protocol {
	case "UDP", "DTLS", "TURN-UDP", "TURN-DTLS":
		_, ok := udpPorts[int(l.Port)]
		udpPorts[int(l.Port)] = true
		return ok
	case "TCP", "TLS", "TURN-TCP", "TURN-TLS":
		_, ok := tcpPorts[int(l.Port)]
		tcpPorts[int(l.Port)] = true
		return ok
	}
	// unknown protocol
	return false
}

// gateway status
func initGatewayStatus(gw *gwapiv1.Gateway, cname string) {
	gw.Status.Addresses = []gwapiv1.GatewayStatusAddress{}

	// set accepted to true and programmed to pending
	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               string(gwapiv1.GatewayConditionAccepted),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1.GatewayReasonAccepted),
		Message: fmt.Sprintf("gateway accepted by controller %s",
			config.ControllerName),
	})

	meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               string(gwapiv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1.GatewayReasonPending),
		Message:            "gateway under processing",
	})

	// reinit listener statuses
	gw.Status.Listeners = gw.Status.Listeners[:0]
	groupgwapiv1a2 := gwapiv1.Group(gwapiv1a2.GroupVersion.Group)
	groupstnrv1 := gwapiv1.Group(stnrgwv1.GroupVersion.Group)

	for _, l := range gw.Spec.Listeners {
		gw.Status.Listeners = append(gw.Status.Listeners,
			gwapiv1.ListenerStatus{
				Name: l.Name,
				SupportedKinds: []gwapiv1.RouteGroupKind{{
					Group: &groupgwapiv1a2,
					Kind:  gwapiv1.Kind("UDPRoute"),
				}, {
					Group: &groupstnrv1,
					Kind:  gwapiv1.Kind("UDPRoute"),
				}},
				Conditions: []metav1.Condition{},
			})
	}
}

func setGatewayStatusProgrammed(gw *gwapiv1.Gateway, err error, pubAddrs []gwAddrPort) {
	if err != nil {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.GatewayReasonInvalid),
			Message: fmt.Sprintf("error processing gateway by controller %q: %s",
				config.ControllerName, err.Error()),
		})
		return
	}

	progd := true
	var gwAddr gwAddrPort
	for _, ap := range pubAddrs {
		if ap.isEmpty() {
			progd = false
		} else {
			gwAddr = ap
		}
	}

	gw.Status.Addresses = []gwapiv1.GatewayStatusAddress{}
	if !gwAddr.isEmpty() {
		aType := gwAddr.aType
		gw.Status.Addresses = []gwapiv1.GatewayStatusAddress{{
			Type:  &aType,
			Value: gwAddr.addr,
		}}
	}

	if progd {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.GatewayConditionProgrammed),
			Message:            "dataplane configuration successfully rendered",
		})
	} else {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.GatewayReasonAddressNotAssigned),
			Message:            "no public address found for at least one listener",
		})
	}
}

// listener status
func getStatus4Listener(gw *gwapiv1.Gateway, l *gwapiv1.Listener) *gwapiv1.ListenerStatus {
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
func setListenerStatus(gw *gwapiv1.Gateway, l *gwapiv1.Listener, err error, conflicted bool, routes int) {
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

func setListenerStatusAccepted(gw *gwapiv1.Gateway, s *gwapiv1.ListenerStatus, reason error) {
	switch {
	case IsNonCriticalError(reason, PortUnavailable):
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonPortUnavailable),
			Message:            "port unavailable",
		})
	case IsNonCriticalError(reason, InvalidProtocol):
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonUnsupportedProtocol),
			Message:            "unsupported protocol",
		})
	case reason != nil:
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionAccepted),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonPending),
			Message:            "pending",
		})
	default:
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonAccepted),
			Message:            "listener accepted",
		})
	}
}

func setListenerStatusConflicted(gw *gwapiv1.Gateway, s *gwapiv1.ListenerStatus, conflicted bool) {
	if !conflicted {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionConflicted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonNoConflicts),
			Message:            "listener protocol-port available",
		})
	} else {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionConflicted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonProtocolConflict),
			Message:            "multiple listeners specified with the same Listener port and protocol",
		})
	}
}

func setListenerStatusResolvedRefs(gw *gwapiv1.Gateway, s *gwapiv1.ListenerStatus) {
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(gwapiv1.ListenerConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1.ListenerReasonResolvedRefs),
		Message:            "listener object references sucessfully resolved",
	})
}

// func setListenerStatusReady(gw *gwapiv1.Gateway, s *gwapiv1.ListenerStatus, ready bool) {
// 	if ready {
// 		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
// 			Type:               string(gwapiv1.ListenerConditionReady),
// 			Status:             metav1.ConditionTrue,
// 			ObservedGeneration: gw.Generation,
// 			LastTransitionTime: metav1.Now(),
// 			Reason:             string(gwapiv1.ListenerReasonReady),
// 			Message:            "public address found for gateway",
// 		})
// 	} else {
// 		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
// 			Type:               string(gwapiv1.ListenerConditionReady),
// 			Status:             metav1.ConditionFalse,
// 			ObservedGeneration: gw.Generation,
// 			LastTransitionTime: metav1.Now(),
// 			Reason:             string(gwapiv1.ListenerReasonPending),
// 			Message:            "public address pending",
// 		})
// 	}
// }
