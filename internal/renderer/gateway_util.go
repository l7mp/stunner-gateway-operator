package renderer

import (
	"errors"
	"fmt"
	"strings"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/config"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"
	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
)

// maxGwayStatusConds is the maximum number of conditions stored in a Gateway status.
const maxGwayStatusConds = 8

type portMap map[int]bool

func (r *renderer) getGateways4Class(c *RenderContext) []*gwapiv1.Gateway {
	ret := []*gwapiv1.Gateway{}

	for _, g := range store.Gateways.GetAll() {
		if string(g.Spec.GatewayClassName) == c.gc.GetName() {
			ret = append(ret, g)
		}
	}

	r.log.V(4).Info("Ready searching gateways for gateway-class",
		"gateway-class", store.GetObjectKey(c.gc), "gateways", len(ret))

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
	proto, _, err := getProtocol(l.Protocol)
	if err != nil {
		// unknown protocol
		return false
	}
	ports := tcpPorts
	if proto == stnrapiv2.ProtocolUDP || proto == stnrapiv2.ProtocolDTLS {
		ports = udpPorts
	}
	_, ok := ports[int(l.Port)]
	ports[int(l.Port)] = true
	return ok
}

// gateway status: set accepted to true and programmed to pending
func initGatewayStatus(gw *gwapiv1.Gateway, reason error) {
	gw.Status.Addresses = []gwapiv1.GatewayStatusAddress{}

	if reason == nil {
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1.GatewayConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.GatewayReasonAccepted),
			Message: fmt.Sprintf("gateway accepted by controller %s",
				config.ControllerName),
		})
	} else {
		cause := gwapiv1.GatewayReasonPending
		if pe := (*ParametersError)(nil); errors.As(reason, &pe) {
			cause = pe.Reason
		}
		meta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1.GatewayConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(cause),
			Message:            reason.Error(),
		})
	}

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
	// RouteGroupKind names a group and a kind, with no version: the official UDPRoute and
	// TCPRoute live in one group whichever version the cluster serves them at.
	groupGwAPI := gwapiv1.Group(gwapiv1.GroupVersion.Group)
	groupStunner := gwapiv1.Group(stnrgwv1.GroupVersion.Group)

	for _, l := range gw.Spec.Listeners {
		gw.Status.Listeners = append(gw.Status.Listeners,
			gwapiv1.ListenerStatus{
				Name: l.Name,
				SupportedKinds: []gwapiv1.RouteGroupKind{{
					Group: &groupGwAPI,
					Kind:  gwapiv1.Kind("UDPRoute"),
				}, {
					Group: &groupStunner,
					Kind:  gwapiv1.Kind("UDPRoute"),
				}, {
					Group: &groupGwAPI,
					Kind:  gwapiv1.Kind("TCPRoute"),
				}, {
					Group: &groupStunner,
					Kind:  gwapiv1.Kind("TCPRoute"),
				}},
				Conditions: []metav1.Condition{},
			})
		// TURNRoute is optional: supported only when the cluster serves its CRD
		if config.TURNRouteServed {
			ls := &gw.Status.Listeners[len(gw.Status.Listeners)-1]
			ls.SupportedKinds = append(ls.SupportedKinds, gwapiv1.RouteGroupKind{
				Group: &groupStunner,
				Kind:  gwapiv1.Kind("TURNRoute"),
			})
		}
	}
}

// setGatewayStatusProgrammed sets the addresses and the Programmed condition of a Gateway: False
// (Invalid) on an error, False (AddressNotAssigned) while a listener has no public address, False
// (Pending) when the rollout of the dataplane Deployment, as last seen by the watch, got stuck,
// True otherwise. A stuck rollout is one whose new pods never became available within the
// progress deadline (unschedulable, image pull, crash loop, failing readiness: the pods tell which)
// or cannot even be created: the old pods keep serving with the previous settings, so the change
// of the pod template never takes effect. A rollout in progress is "ready soon" and a DaemonSet
// does not report a stuck rollout, so neither turns the condition False.
func setGatewayStatusProgrammed(gw *gwapiv1.Gateway, err error, pubAddrs []gwAddrPort) {
	cond := metav1.Condition{
		Type:               string(gwapiv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
	}
	if err != nil {
		cond.Reason = string(gwapiv1.GatewayReasonInvalid)
		cond.Message = fmt.Sprintf("error processing gateway by controller %s: %s",
			config.ControllerName, err.Error())
		meta.SetStatusCondition(&gw.Status.Conditions, cond)
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

	stuck := ""
	if dep := store.Deployments.GetObject(store.GetNamespacedName(gw)); dep != nil && !isManagedDataplaneDisabled(gw) {
		for _, c := range dep.Status.Conditions {
			switch {
			case c.Type == appv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue:
				stuck = "the dataplane pods cannot be created: " + c.Message
			case c.Type == appv1.DeploymentProgressing && c.Status == corev1.ConditionFalse && stuck == "":
				stuck = "the new dataplane pods did not become available within the progress " +
					"deadline, the old pods keep serving with the previous settings (the pods tell why)"
			}
		}
	}

	switch {
	case !progd:
		cond.Reason = string(gwapiv1.GatewayReasonAddressNotAssigned)
		cond.Message = "no public address found for at least one listener"
	case stuck != "":
		cond.Reason = string(gwapiv1.GatewayReasonPending)
		cond.Message = stuck
	default:
		cond.Status = metav1.ConditionTrue
		cond.Reason = string(gwapiv1.GatewayConditionProgrammed)
		cond.Message = "dataplane configuration successfully rendered"
	}
	meta.SetStatusCondition(&gw.Status.Conditions, cond)
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

// setListenerStatus sets the Accepted, Conflicted and ResolvedRefs conditions and the number of
// attached routes of a listener.
func setListenerStatus(gw *gwapiv1.Gateway, l *gwapiv1.Listener, err error, conflicted bool, routes int) {
	s := getStatus4Listener(gw, l)
	if s == nil {
		// should never happen
		return
	}

	setListenerStatusAccepted(gw, s, err)
	setListenerStatusConflicted(gw, s, conflicted)
	setListenerStatusResolvedRefs(gw, s, err)
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
	case IsNonCriticalError(reason, UnsupportedSRT):
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonUnsupportedProtocol),
			Message:            reason.Error(),
		})
	case IsNonCriticalError(reason, InvalidCertificateRef):
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonAccepted),
			Message:            "listener accepted",
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

func setListenerStatusResolvedRefs(gw *gwapiv1.Gateway, s *gwapiv1.ListenerStatus, reason error) {
	if IsNonCriticalError(reason, InvalidCertificateRef) {
		meta.SetStatusCondition(&s.Conditions, metav1.Condition{
			Type:               string(gwapiv1.ListenerConditionResolvedRefs),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gw.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.ListenerReasonInvalidCertificateRef),
			Message:            "at least one certificate reference failed to be successfully resolved",
		})
		return
	}

	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(gwapiv1.ListenerConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gw.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gwapiv1.ListenerReasonResolvedRefs),
		Message:            "listener object references successfully resolved",
	})
}
