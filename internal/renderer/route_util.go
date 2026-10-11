package renderer

import (
	"fmt"

	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// allRoutes returns all routes of all route kinds, skipping the Gateway API routes that are
// masked by a same-namespace/name STUNner-native route.
func (r *renderer) allRoutes() []store.Route {
	ret := []store.Route{}

	for _, ro := range store.UDPRoutes.GetAll() {
		ret = append(ret, ro)
	}

	for _, ro := range store.UDPRoutesGwAPI.GetAll() {
		if isRouteMasked(ro) {
			r.log.Info("Ignoring Gateway API UDPRoute masked by a STUNner UDPRoute",
				"route", store.GetObjectKey(ro))
			continue
		}
		ret = append(ret, ro)
	}

	for _, ro := range store.TCPRoutes.GetAll() {
		ret = append(ret, ro)
	}

	for _, ro := range store.TCPRoutesGwAPI.GetAll() {
		if isRouteMasked(ro) {
			r.log.Info("Ignoring Gateway API TCPRoute masked by a STUNner TCPRoute",
				"route", store.GetObjectKey(ro))
			continue
		}
		ret = append(ret, ro)
	}

	for _, ro := range store.TURNRoutes.GetAll() {
		ret = append(ret, ro)
	}

	return ret
}

func (r *renderer) getRoutes4Listener(gw *gwapiv1.Gateway, l *gwapiv1.Listener) []store.Route {
	r.log.V(4).Info("getRoutes4Listener", "gateway", store.GetObjectKey(gw), "listener", l.Name)

	ret := make([]store.Route, 0)
	rs := r.allRoutes()
	for i := range rs {
		ro := rs[i]
		r.log.V(4).Info("Considering route for listener", "gateway",
			store.GetObjectKey(gw), "listener", l.Name, "route",
			store.GetObjectKey(ro))

		parents := ro.GetParentRefs()
		for j := range parents {
			p := parents[j]

			found, reason := resolveParentRef(ro, &p, gw, l)
			if !found {
				r.log.V(4).Info("Route parent rejected for listener",
					"gateway", store.GetObjectKey(gw), "listener", l.Name,
					"route", store.GetObjectKey(ro), "parent", store.DumpParentRef(&p),
					"reason", reason)

				continue
			}

			r.log.V(4).Info("Route found", "gateway",
				store.GetObjectKey(gw), "listener", l.Name, "route",
				store.GetObjectKey(ro))

			// route made it this far: attach once, whatever other parents match
			ret = append(ret, ro)
			break
		}
	}

	return ret
}

func resolveParentRef(ro store.Route, p *gwapiv1.ParentReference, gw *gwapiv1.Gateway, l *gwapiv1.Listener) (bool, string) {
	if p.Group != nil && *p.Group != gwapiv1.Group(gwapiv1.GroupVersion.Group) {
		return false, fmt.Sprintf("parent group %q does not match gateway group %q",
			string(*p.Group), gwapiv1.GroupVersion.Group)
	}

	if p.Kind != nil && *p.Kind != "Gateway" {
		return false, fmt.Sprintf("parent kind %q does not match gateway kind %q",
			string(*p.Kind), "Gateway")
	}

	namespace := gwapiv1.Namespace(ro.GetNamespace())
	if p.Namespace != nil {
		namespace = *p.Namespace
	}
	if namespace != gwapiv1.Namespace(gw.GetNamespace()) {
		return false, fmt.Sprintf("parent namespace %q does not match gateway namespace %q",
			string(namespace), gw.GetNamespace())
	}

	if p.Name != gwapiv1.ObjectName(gw.GetName()) {
		return false, fmt.Sprintf("parent name %q does not match gateway name %q",
			string(p.Name), gw.GetName())
	}
	allowed, msg := gatewayAllowsNamespace(ro, gw, l)
	if !allowed {
		return false, msg
	}

	allowed, msg = gatewayAllowsKind(ro, l)
	if !allowed {
		return false, msg
	}

	if p.SectionName != nil && *p.SectionName != l.Name {
		return false, fmt.Sprintf("parent SectionName %q does not match listener name %q",
			string(*p.SectionName), l.Name)
	}

	return true, ""
}

func gatewayAllowsNamespace(ro store.Route, gw *gwapiv1.Gateway, l *gwapiv1.Listener) (bool, string) {
	// default namespace attachment policy: Same
	if l.AllowedRoutes == nil || l.AllowedRoutes.Namespaces == nil || l.AllowedRoutes.Namespaces.From == nil {
		return gatewayAllowsSameNamespace(ro, gw)
	}

	allowedNamespaces := l.AllowedRoutes.Namespaces
	switch *allowedNamespaces.From {
	case gwapiv1.NamespacesFromAll:
		return true, ""
	case gwapiv1.NamespacesFromSelector:
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
				"find namespace %q for route %q in local storage", store.GetObjectKey(gw),
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

func gatewayAllowsSameNamespace(ro store.Route, gw *gwapiv1.Gateway) (bool, string) {
	allowed := gw.GetNamespace() == ro.GetNamespace()
	if !allowed {
		return false, fmt.Sprintf("parent %q/%q (namespace attachment policy: Same) rejects route %q/%q",
			gw.GetName(), gw.GetNamespace(), ro.GetName(), ro.GetNamespace())
	}
	return true, ""
}

// gatewayAllowsKind checks the route kind against the AllowedRoutes.Kinds attachment policy of
// the listener. An empty policy admits every route kind.
func gatewayAllowsKind(ro store.Route, l *gwapiv1.Listener) (bool, string) {
	if l.AllowedRoutes == nil || len(l.AllowedRoutes.Kinds) == 0 {
		return true, ""
	}

	kind := routeKind(ro)
	for _, k := range l.AllowedRoutes.Kinds {
		if k.Kind != kind {
			continue
		}
		// both the Gateway API and the STUNner group are accepted, an unset group
		// defaults to the Gateway API group
		if k.Group == nil || *k.Group == gwapiv1.Group(gwapiv1.GroupVersion.Group) ||
			*k.Group == gwapiv1.Group(stnrgwv1.GroupVersion.Group) {
			return true, ""
		}
	}

	return false, fmt.Sprintf("listener %q (route kind attachment policy) rejects route kind %q",
		l.Name, kind)
}

// routeKind returns the Gateway API kind of a route.
func routeKind(ro store.Route) gwapiv1.Kind {
	switch ro.(type) {
	case *stnrgwv1.UDPRoute:
		return gwapiv1.Kind("UDPRoute")
	case *stnrgwv1.TCPRoute:
		return gwapiv1.Kind("TCPRoute")
	case *stnrgwv1a1.TURNRoute:
		return gwapiv1.Kind("TURNRoute")
	default:
		return gwapiv1.Kind("")
	}
}

func initRouteStatus(ro store.Route) {
	ro.GetRouteStatus().Parents = []gwapiv1.RouteParentStatus{}
}

// isRouteControlled returns true if at least one of the parents of the route is controlled by us.
func (r *renderer) isRouteControlled(ro store.Route) bool {
	gcs := r.getGatewayClasses()

	parents := ro.GetParentRefs()
	for i := range parents {
		p := &parents[i]

		// obtain the parent gw
		gw := r.getParentGateway(ro, p)
		if gw == nil {
			continue
		}

		// obtain the gatewayclass
		for _, gc := range gcs {
			if gc.GetName() == string(gw.Spec.GatewayClassName) {
				r.log.V(2).Info("Route is handled by this controller: accepting",
					"route", store.GetObjectKey(ro),
					"parent", store.DumpParentRef(p),
					"linked-gateway-class", gw.Spec.GatewayClassName,
				)
				return true
			}
		}
	}

	r.log.V(2).Info("Route is handled by another controller: rejecting",
		"route", store.GetObjectKey(ro))

	return false
}

// isParentOutContext returns true if (1) the parent exists and (2) it is NOT included in the
// gateway context being processed (in which case we do not generate a status for the parent)
func (r *renderer) isParentOutContext(gws *store.GatewayStore, ro store.Route, p *gwapiv1.ParentReference) bool {
	// find the corresponding gateway
	ns := ro.GetNamespace()
	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: string(p.Name)}
	ret := store.Gateways.GetObject(namespacedName) != nil && gws.GetObject(namespacedName) == nil

	r.log.V(4).Info("Parent context check ready", "route", store.GetObjectKey(ro),
		"parent", store.DumpParentRef(p), "gw-context-length", gws.Len(), "result", ret)

	return ret
}

// isParentAcceptingRoute decides whether a parent accepts a route:
//
// - returns (exists, accepted)=(bool,bool) indicating whether the parent exists and if it is,
// whether it accepts the route
//
// - arg className == "" means "do not consider classness of parent", this is useful for generating
// a route status that is consistent across rendering contexts
func (r *renderer) isParentAcceptingRoute(ro store.Route, p *gwapiv1.ParentReference, className string) (bool, bool) {
	gw := r.getParentGateway(ro, p)
	if gw == nil {
		r.log.V(4).Info("No gateway found for parent", "route",
			store.GetObjectKey(ro), "parent", store.DumpParentRef(p))
		return false, false
	}

	// does the parent belong to the class we are processing: we don't want to generate routes
	// for gateways that link to other classes
	if className != "" && gw.Spec.GatewayClassName != gwapiv1.ObjectName(className) {
		r.log.V(4).Info("Parent links to a gateway that is being managed by another "+
			"gateway-class: rejecting", "route", store.GetObjectKey(ro), "parent",
			store.DumpParentRef(p), "linked-gateway-class", gw.Spec.GatewayClassName,
			"current-gateway-class", className)
		return false, false
	}

	// is there a listener that accepts us?
	for i := range gw.Spec.Listeners {
		l := gw.Spec.Listeners[i]

		found, msg := resolveParentRef(ro, p, gw, &l)
		if found {
			r.log.V(3).Info("Gateway and/or listener found for parent",
				"route", store.GetObjectKey(ro), "parent", store.DumpParentRef(p),
				"gateway", gw.GetName(), "listener", l.Name)

			return true, true
		} else {
			r.log.V(4).Info("Gateway and/or listener does not accept route",
				"route", store.GetObjectKey(ro), "parent", store.DumpParentRef(p),
				"gateway", gw.GetName(), "listener", l.Name, "message", msg)
		}
	}

	r.log.V(4).Info("Checked route parent ready", "route", store.GetObjectKey(ro),
		"parent", fmt.Sprintf("%#v", *p), "result", "rejected")

	return true, false
}

func (r *renderer) getParentGateway(ro store.Route, p *gwapiv1.ParentReference) *gwapiv1.Gateway {
	// find the corresponding gateway
	ns := ro.GetNamespace()
	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: string(p.Name)}
	return store.Gateways.GetObject(namespacedName)
}

// invalidateMaskedRoutes invalidates the masked Gateway API routes.
func (r *renderer) invalidateMaskedRoutes(c *RenderContext) {
	maskable := []store.Route{}
	for _, ro := range store.UDPRoutesGwAPI.GetAll() {
		maskable = append(maskable, ro)
	}
	for _, ro := range store.TCPRoutesGwAPI.GetAll() {
		maskable = append(maskable, ro)
	}

	for _, ro := range maskable {
		if !isRouteMasked(ro) || !r.isRouteControlled(ro) {
			continue
		}

		initRouteStatus(ro)
		parents := ro.GetParentRefs()
		for i := range parents {
			p := parents[i]
			parentExists, parentAccept := r.isParentAcceptingRoute(ro, &p, "")
			// automatically handles masked routes
			setRouteConditionStatus(ro, &p, config.ControllerName, parentExists, parentAccept, nil)
		}

		queueRouteStatusUpdate(c, ro)
	}
}

// queueRouteStatusUpdate schedules a route for a status update on the queue that matches the
// route's original API object kind. Note that the same route may be processed several times, in
// the context of different Gateways: Upsert makes sure the last render will be updated.
func queueRouteStatusUpdate(c *RenderContext, ro store.Route) {
	switch ro := ro.(type) {
	case *stnrgwv1.UDPRoute:
		switch {
		case !isRouteGwAPIUDP(ro):
			c.update.UpsertQueue.UDPRoutes.Upsert(ro.DeepCopy())
		case config.GwAPIUDPRouteVersion == config.GwAPIVersionV1A2:
			c.update.UpsertQueue.UDPRoutesGwAPI.Upsert(statusTargetV1A2UDPRoute(ro))
		default:
			c.update.UpsertQueue.UDPRoutesGwAPI.Upsert(statusTargetV1UDPRoute(ro))
		}
	case *stnrgwv1.TCPRoute:
		switch {
		case !isRouteGwAPITCP(ro):
			c.update.UpsertQueue.TCPRoutes.Upsert(ro.DeepCopy())
		case config.GwAPITCPRouteVersion == config.GwAPIVersionV1A2:
			c.update.UpsertQueue.TCPRoutesGwAPI.Upsert(statusTargetV1A2TCPRoute(ro))
		default:
			c.update.UpsertQueue.TCPRoutesGwAPI.Upsert(statusTargetV1TCPRoute(ro))
		}
	case *stnrgwv1a1.TURNRoute:
		c.update.UpsertQueue.TURNRoutes.Upsert(ro.DeepCopy())
	}
}

// statusTargetV1A2UDPRoute builds a v1alpha2 bearer object for status updates.
//
// Rendering uses STUNner v1 routes as the canonical in-memory representation,
// including routes converted from the official Gateway API versions. The updater, however, must
// call Status().Update on the concrete API object type that exists in the
// cluster. This adapter converts canonical route status to that bearer type.
func statusTargetV1A2UDPRoute(ro *stnrgwv1.UDPRoute) *gwapiv1a2.UDPRoute {
	ret := &gwapiv1a2.UDPRoute{}
	ret.SetName(ro.GetName())
	ret.SetNamespace(ro.GetNamespace())
	ro.Status.RouteStatus.DeepCopyInto(&ret.Status.RouteStatus)
	return ret
}

// statusTargetV1UDPRoute builds a graduated Gateway API v1 bearer object for UDPRoute status
// updates (see statusTargetV1A2UDPRoute).
func statusTargetV1UDPRoute(ro *stnrgwv1.UDPRoute) *gwapiv1.UDPRoute {
	ret := &gwapiv1.UDPRoute{}
	ret.SetName(ro.GetName())
	ret.SetNamespace(ro.GetNamespace())
	ro.Status.RouteStatus.DeepCopyInto(&ret.Status.RouteStatus)
	return ret
}

// statusTargetV1A2TCPRoute builds a deprecated Gateway API v1alpha2 bearer object for TCPRoute
// status updates (see statusTargetV1A2UDPRoute).
func statusTargetV1A2TCPRoute(ro *stnrgwv1.TCPRoute) *gwapiv1a2.TCPRoute {
	ret := &gwapiv1a2.TCPRoute{}
	ret.SetName(ro.GetName())
	ret.SetNamespace(ro.GetNamespace())
	ro.Status.RouteStatus.DeepCopyInto(&ret.Status.RouteStatus)
	return ret
}

// statusTargetV1TCPRoute builds a Gateway API v1 bearer object for TCPRoute status updates (see
// statusTargetV1A2UDPRoute).
func statusTargetV1TCPRoute(ro *stnrgwv1.TCPRoute) *gwapiv1.TCPRoute {
	ret := &gwapiv1.TCPRoute{}
	ret.SetName(ro.GetName())
	ret.SetNamespace(ro.GetNamespace())
	ro.Status.RouteStatus.DeepCopyInto(&ret.Status.RouteStatus)
	return ret
}

func setRouteConditionStatus(ro store.Route, p *gwapiv1.ParentReference, controllerName string, exists, accepted bool, backendErr error) {
	pRef := gwapiv1.ParentReference{
		Name: p.Name,
	}

	if p.Group != nil && *p.Group != gwapiv1.Group(gwapiv1.GroupVersion.Group) {
		pRef.Group = p.Group
	}

	if p.Kind != nil && *p.Kind != "Gateway" {
		pRef.Kind = p.Kind
	}

	if p.Namespace != nil {
		pRef.Namespace = p.Namespace
	}

	if p.SectionName != nil {
		pRef.SectionName = p.SectionName
	}

	s := gwapiv1.RouteParentStatus{
		ParentRef:      pRef,
		ControllerName: gwapiv1.GatewayController(controllerName),
		Conditions:     []metav1.Condition{},
	}

	if isRouteMasked(ro) {
		setRouteAcceptedCondition(ro, &s.Conditions, gwapiv1.RouteReasonPending,
			metav1.ConditionFalse, "Gateway API route masked by a STUNner-native route")
	} else {
		namespace := ro.GetNamespace()
		if p.Namespace != nil {
			namespace = string(*p.Namespace)
		}
		id := fmt.Sprintf("%s/%s", namespace, string(p.Name))
		if exists {
			if accepted {
				setRouteAcceptedCondition(ro, &s.Conditions, gwapiv1.RouteReasonAccepted,
					metav1.ConditionTrue, fmt.Sprintf("parent %s accepted the route", id))
			} else {
				setRouteAcceptedCondition(ro, &s.Conditions, gwapiv1.RouteReasonNotAllowedByListeners,
					metav1.ConditionFalse, fmt.Sprintf("parent %s rejects the route", id))
			}
		} else {
			setRouteAcceptedCondition(ro, &s.Conditions, gwapiv1.RouteReasonNoMatchingParent,
				metav1.ConditionFalse, fmt.Sprintf("parent %s does not exist", id))
		}
	}

	var resolvedCond metav1.Condition
	if backendErr != nil {
		var reason gwapiv1.RouteConditionReason
		message := "at least one backend reference failed to be successfully resolved"
		switch {
		case IsNonCriticalError(backendErr, InvalidBackendKind), IsNonCriticalError(backendErr, InvalidBackendGroup):
			// "RouteReasonInvalidKind" is used with the "ResolvedRefs" condition when
			// one of the Route's rules has a reference to an unknown or unsupported
			// Group and/or Kind.
			reason = gwapiv1.RouteReasonInvalidKind
		case IsNonCriticalError(backendErr, FeatureNotLicensed):
			// the route kind itself is unavailable in the current license tier, so no
			// cluster is rendered for it at all
			reason = gwapiv1.RouteReasonUnsupportedValue
			message = "route kind not available in the current license tier"
		default:
			reason = gwapiv1.RouteReasonBackendNotFound
		}
		resolvedCond = metav1.Condition{
			Type:               string(gwapiv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ro.GetGeneration(),
			LastTransitionTime: metav1.Now(),
			Reason:             string(reason),
			Message:            message,
		}
	} else {
		resolvedCond = metav1.Condition{
			Type:               string(gwapiv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: ro.GetGeneration(),
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.RouteReasonResolvedRefs),
			Message:            "all backend references successfully resolved",
		}
	}

	meta.SetStatusCondition(&s.Conditions, resolvedCond)

	status := ro.GetRouteStatus()
	status.Parents = append(status.Parents, s)
}

func setRouteAcceptedCondition(ro store.Route, s *[]metav1.Condition, reason gwapiv1.RouteConditionReason, status metav1.ConditionStatus, message string) {
	meta.SetStatusCondition(s, metav1.Condition{
		Type:               string(gwapiv1.RouteConditionAccepted),
		Status:             status,
		ObservedGeneration: ro.GetGeneration(),
		LastTransitionTime: metav1.Now(),
		Reason:             string(reason),
		Message:            message,
	})
}

// check by pointer: namespacedname is not unique across STUNner and Gateway API routes
func isRouteGwAPIUDP(ro client.Object) bool {
	return store.UDPRoutesGwAPI.Get(store.GetNamespacedName(ro)) == ro
}

// check by pointer: namespacedname is not unique across STUNner and Gateway API routes
func isRouteGwAPITCP(ro client.Object) bool {
	return store.TCPRoutesGwAPI.Get(store.GetNamespacedName(ro)) == ro
}

// isRouteMasked returns true for a Gateway API route that is overridden by a STUNner-native route
// of the same kind, namespace and name.
func isRouteMasked(ro client.Object) bool {
	switch {
	case isRouteGwAPIUDP(ro):
		return store.UDPRoutes.Get(store.GetNamespacedName(ro)) != nil
	case isRouteGwAPITCP(ro):
		return store.TCPRoutes.Get(store.GetNamespacedName(ro)) != nil
	default:
		return false
	}
}
