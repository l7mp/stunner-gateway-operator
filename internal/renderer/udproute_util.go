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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) getUDPRoutes4Listener(gw *gwapiv1b1.Gateway, l *gwapiv1b1.Listener) []*gwapiv1a2.UDPRoute {
	r.log.V(4).Info("getUDPRoutes4Listener", "gateway", store.GetObjectKey(gw), "listener",
		l.Name)

	ret := make([]*gwapiv1a2.UDPRoute, 0)
	rs := store.UDPRoutes.GetAll()

	for i := range rs {
		ro := rs[i]
		r.log.V(4).Info("getUDPRoutes4Listener: considering route for listener", "gateway",
			store.GetObjectKey(gw), "listener", l.Name, "route",
			store.GetObjectKey(ro))

		for j := range ro.Spec.CommonRouteSpec.ParentRefs {
			p := ro.Spec.CommonRouteSpec.ParentRefs[j]

			found, reason := resolveParentRef(ro, &p, gw, l)
			if !found {
				r.log.V(4).Info("getUDPRoutes4Listener: parent rejected for listener",
					"gateway", store.GetObjectKey(gw), "listener", l.Name,
					"route", store.GetObjectKey(ro), "parent", dumpParentRef(&p),
					"reason", reason)

				continue
			}

			r.log.V(4).Info("getUDPRoutes4Listener: route found", "gateway",
				store.GetObjectKey(gw), "listener", l.Name, "route",
				store.GetObjectKey(ro))

			// route made it this far: attach!
			ret = append(ret, ro)
		}

	}

	return ret
}

func resolveParentRef(ro *gwapiv1a2.UDPRoute, p *gwapiv1b1.ParentReference, gw *gwapiv1b1.Gateway, l *gwapiv1b1.Listener) (bool, string) {
	if p.Group != nil && *p.Group != gwapiv1b1.Group(gwapiv1b1.GroupVersion.Group) {
		return false, fmt.Sprintf("parent group %q does not match gateway group %q",
			string(*p.Group), gwapiv1b1.GroupVersion.Group)
	}

	if p.Kind != nil && *p.Kind != "Gateway" {
		return false, fmt.Sprintf("parent kind %q does not match gateway kind %q",
			string(*p.Kind), "Gateway")
	}

	namespace := gwapiv1b1.Namespace(ro.GetNamespace())
	if p.Namespace != nil {
		namespace = *p.Namespace
	}
	if namespace != gwapiv1b1.Namespace(gw.GetNamespace()) {
		return false, fmt.Sprintf("parent namespace %q does not match gateway namespace %q",
			string(namespace), gw.GetNamespace())
	}

	if p.Name != gwapiv1b1.ObjectName(gw.GetName()) {
		return false, fmt.Sprintf("parent name %q does not match gateway name %q",
			string(p.Name), gw.GetName())
	}
	allowed, msg := gatewayAllowsNamespace(ro, gw, l)
	if !allowed {
		return false, msg
	}

	if p.SectionName != nil && *p.SectionName != l.Name {
		return false, fmt.Sprintf("parent SectionName %q does not match listener name %q",
			string(*p.SectionName), l.Name)
	}

	return true, ""
}

func gatewayAllowsNamespace(ro *gwapiv1a2.UDPRoute, gw *gwapiv1b1.Gateway, l *gwapiv1b1.Listener) (bool, string) {
	// default namespace attachment policy: Same
	if l.AllowedRoutes == nil || l.AllowedRoutes.Namespaces == nil || l.AllowedRoutes.Namespaces.From == nil {
		return gatewayAllowsSameNamespace(ro, gw)
	}

	allowedNamespaces := l.AllowedRoutes.Namespaces
	switch *allowedNamespaces.From {
	case gwapiv1b1.NamespacesFromAll:
		return true, ""
	case gwapiv1b1.NamespacesFromSelector:
		if allowedNamespaces.Selector == nil {
			return false, fmt.Sprintf("parent %s (namespace attachment policy: Selector): Selector missing",
				store.GetObjectKey(gw))
		}
		selector, err := metav1.LabelSelectorAsSelector(allowedNamespaces.Selector)
		if err != nil {
			return false, fmt.Sprintf("parent %s (namespace attachment policy: Selector): cannot create selector: %s",
				store.GetObjectKey(gw), err.Error())

		}
		// get the namespace of the route
		ns := types.NamespacedName{Name: ro.GetNamespace()}
		namespace := store.Namespaces.GetObject(ns)
		if namespace == nil {
			return false, fmt.Sprintf("parent %s (namespace attachment policy: Selector): cannot "+
				"find namespace %q for udproute %q in local storage", store.GetObjectKey(gw),
				store.GetObjectKey(ro), ns.String())
		}
		res := selector.Matches(labels.Set(namespace.Labels))
		if !res {
			return false, fmt.Sprintf("parent %s (namespace attachment policy: Selector): labels on "+
				"namespace %q do not match Selector", store.GetObjectKey(gw), ns.String())
		}
		return true, ""
	default:
		// NamespacesFromSame is the default
		return gatewayAllowsSameNamespace(ro, gw)
	}
}

func gatewayAllowsSameNamespace(ro *gwapiv1a2.UDPRoute, gw *gwapiv1b1.Gateway) (bool, string) {
	allowed := gw.GetNamespace() == ro.GetNamespace()
	if !allowed {
		return false, fmt.Sprintf("parent %q/%q (namespace attachment policy: Same) rejects route %q/%q",
			gw.GetName(), gw.GetNamespace(), ro.GetName(), ro.GetNamespace())
	}
	return true, ""
}

func initRouteStatus(ro *gwapiv1a2.UDPRoute) {
	ro.Status.Parents = []gwapiv1b1.RouteParentStatus{}
}

// isParentController returns true if at least one of the parents of the route is controlled by us
func (r *Renderer) isRouteControlled(ro *gwapiv1a2.UDPRoute) bool {
	gcs := r.getGatewayClasses()

	for i := range ro.Spec.ParentRefs {
		p := &ro.Spec.ParentRefs[i]

		// obtain the parent gw
		gw := r.getParentGateway(ro, p)
		if gw == nil {
			continue
		}

		// obtain the gatewayclass
		for _, gc := range gcs {
			if gc.GetName() == string(gw.Spec.GatewayClassName) {
				r.log.V(2).Info("route is handled by this controller: accepting",
					"route", store.GetObjectKey(ro),
					"parent", dumpParentRef(p),
					"linked-gateway-class", gw.Spec.GatewayClassName,
				)
				return true
			}
		}
	}

	r.log.V(2).Info("route is handled by another controller: rejecting",
		"route", store.GetObjectKey(ro),
	)

	return false
}

// isParentOutContext returns true if (1) the parent exists and (2) it is NOT included in the
// gateway context being processed (in which case we do not generate a status for the parent)
func (r *Renderer) isParentOutContext(gws *store.GatewayStore, ro *gwapiv1a2.UDPRoute, p *gwapiv1b1.ParentReference) bool {
	// find the corresponding gateway
	ns := ro.GetNamespace()
	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: string(p.Name)}
	ret := store.Gateways.GetObject(namespacedName) != nil && gws.GetObject(namespacedName) == nil

	r.log.V(4).Info("isParentOutContext", "route", store.GetObjectKey(ro),
		"parent", dumpParentRef(p), "gw-context-length", gws.Len(), "result", ret)

	return ret
}

// className == "" means "do not consider classness of parent", this is useful for generating a
// route status that is consistent across rendering contexts
func (r *Renderer) isParentAcceptingRoute(ro *gwapiv1a2.UDPRoute, p *gwapiv1b1.ParentReference, className string) bool {
	// r.log.V(4).Info("isParentAcceptingRoute", "route", store.GetObjectKey(ro),
	// 	"parent", dumpParentRef(p))

	gw := r.getParentGateway(ro, p)
	if gw == nil {
		r.log.V(4).Info("no gateway found for Parent", "route",
			store.GetObjectKey(ro), "parent", dumpParentRef(p))
		return false
	}

	// does the parent belong to the class we are processing: we don't want to generate routes
	// for gateways that link to other classes
	if className != "" && gw.Spec.GatewayClassName != gwapiv1b1.ObjectName(className) {
		r.log.V(4).Info("parent links to a gateway that is being managed by another "+
			"gateway-class: rejecting", "route", store.GetObjectKey(ro), "parent",
			dumpParentRef(p), "linked-gateway-class", gw.Spec.GatewayClassName,
			"current-gateway-class", className)
		return false
	}

	// is there a listener that accepts us?
	for i := range gw.Spec.Listeners {
		l := gw.Spec.Listeners[i]

		found, msg := resolveParentRef(ro, p, gw, &l)
		if found {
			r.log.V(3).Info("isParentAcceptingRoute: gateway/listener found for parent",
				"route", store.GetObjectKey(ro), "parent", dumpParentRef(p),
				"gateway", gw.GetName(), "listener", l.Name)

			return true
		} else {
			r.log.V(4).Info("isParentAcceptingRoute: gateway/listener does not accept route",
				"route", store.GetObjectKey(ro), "parent", dumpParentRef(p),
				"gateway", gw.GetName(), "listener", l.Name, "message", msg)
		}
	}

	r.log.V(4).Info("isParentAcceptingRoute result", "route", store.GetObjectKey(ro),
		"parent", fmt.Sprintf("%#v", *p), "result", "rejected")

	return false
}

func (r *Renderer) getParentGateway(ro *gwapiv1a2.UDPRoute, p *gwapiv1b1.ParentReference) *gwapiv1b1.Gateway {
	// find the corresponding gateway
	ns := ro.GetNamespace()
	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: string(p.Name)}
	return store.Gateways.GetObject(namespacedName)
}

func setRouteConditionStatus(ro *gwapiv1a2.UDPRoute, p *gwapiv1b1.ParentReference, controllerName string, accepted bool, backendErr error) {
	// ns := gwapiv1b1.Namespace(ro.GetNamespace())
	// gr := gwapiv1b1.Group(gwapiv1b1.GroupVersion.Group)
	// kind := gwapiv1b1.Kind("Gateway")

	pRef := gwapiv1b1.ParentReference{
		Name: p.Name,
	}

	if p.Group != nil && *p.Group != gwapiv1b1.Group(gwapiv1b1.GroupVersion.Group) {
		pRef.Group = p.Group
	}

	if p.Kind != nil && *p.Kind != "Gateway" {
		pRef.Kind = p.Kind
	}

	// if p.Namespace != nil && *p.Namespace != gwapiv1a2.Namespace(ro.GetNamespace()) {
	if p.Namespace != nil {
		pRef.Namespace = p.Namespace
	}

	if p.SectionName != nil {
		pRef.SectionName = p.SectionName
	}

	s := gwapiv1b1.RouteParentStatus{
		ParentRef:      pRef,
		ControllerName: gwapiv1b1.GatewayController(controllerName),
		Conditions:     []metav1.Condition{},
	}

	var acceptCond metav1.Condition
	if accepted {
		acceptCond = metav1.Condition{
			Type:               string(gwapiv1b1.RouteConditionAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: ro.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.RouteReasonAccepted),
			Message:            "parent accepts the route",
		}
	} else {
		acceptCond = metav1.Condition{
			Type:               string(gwapiv1b1.RouteConditionAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ro.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.RouteReasonNotAllowedByListeners),
			Message:            "parent rejects the route",
		}
	}
	meta.SetStatusCondition(&s.Conditions, acceptCond)

	var resolvedCond metav1.Condition
	if backendErr != nil {
		var reason gwapiv1b1.RouteConditionReason
		switch {
		case IsNonCriticalError(backendErr, InvalidBackendKind), IsNonCriticalError(backendErr, InvalidBackendGroup):
			// "RouteReasonInvalidKind" is used with the "ResolvedRefs" condition when
			// one of the Route's rules has a reference to an unknown or unsupported
			// Group and/or Kind.
			reason = gwapiv1b1.RouteReasonInvalidKind
		default:
			reason = gwapiv1b1.RouteReasonBackendNotFound
		}
		resolvedCond = metav1.Condition{
			Type:               string(gwapiv1b1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ro.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(reason),
			Message:            "at least one backend reference failed to be successfully resolved",
		}
	} else {
		resolvedCond = metav1.Condition{
			Type:               string(gwapiv1b1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: ro.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.RouteReasonResolvedRefs),
			Message:            "all backend references successfully resolved",
		}
	}

	meta.SetStatusCondition(&s.Conditions, resolvedCond)

	ro.Status.Parents = append(ro.Status.Parents, s)
}

func dumpParentRef(p *gwapiv1b1.ParentReference) string {
	g, k, ns, sn := "<NIL>", "<NIL>", "<NIL>", "<NIL>"
	if p.Group != nil {
		g = string(*p.Group)
	}

	if p.Kind != nil {
		k = string(*p.Kind)
	}

	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	if p.SectionName != nil {
		sn = string(*p.SectionName)
	}

	return fmt.Sprintf("{Group: %s, Kind: %s, Namespace: %s, Name: %s, SectionName: %s}",
		g, k, ns, p.Name, sn)
}

func dumpBackendRef(b *gwapiv1b1.BackendRef) string {
	g, k, ns := "<NIL>", "<NIL>", "<NIL>"
	if b.Group != nil {
		g = string(*b.Group)
	}

	if b.Kind != nil {
		k = string(*b.Kind)
	}

	if b.Namespace != nil {
		ns = string(*b.Namespace)
	}

	return fmt.Sprintf("{Group: %s, Kind: %s, Namespace: %s, Name: %s}",
		g, k, ns, b.Name)
}
