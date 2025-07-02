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

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *renderer) allUDPRoutes() []*stnrgwv1.UDPRoute {
	rs := store.UDPRoutes.GetAll()

	for _, uv1a2 := range store.UDPRoutesV1A2.GetAll() {
		if isRouteMasked(uv1a2) {
			r.log.Info("Ignoring gwapiv1a2.UDPRoute masking a stunnerv1.UDPRoute:",
				"name", uv1a2.GetName(), "namespace", uv1a2.GetNamespace())
			continue
		}
		rs = append(rs, uv1a2)
	}

	return rs
}

func (r *renderer) getUDPRoutes4Listener(gw *gwapiv1.Gateway, l *gwapiv1.Listener) []*stnrgwv1.UDPRoute {
	r.log.V(4).Info("getUDPRoutes4Listener", "gateway", store.GetObjectKey(gw), "listener", l.Name)

	ret := make([]*stnrgwv1.UDPRoute, 0)
	rs := r.allUDPRoutes()
	for i := range rs {
		ro := rs[i]
		r.log.V(4).Info("Considering route for listener", "gateway",
			store.GetObjectKey(gw), "listener", l.Name, "route",
			store.GetObjectKey(ro))

		for j := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[j]

			found, reason := resolveParentRef(ro, &p, gw, l)
			if !found {
				r.log.V(4).Info("UDPRoute parent rejected for listener",
					"gateway", store.GetObjectKey(gw), "listener", l.Name,
					"route", store.GetObjectKey(ro), "parent", store.DumpParentRef(&p),
					"reason", reason)

				continue
			}

			r.log.V(4).Info("Route found", "gateway",
				store.GetObjectKey(gw), "listener", l.Name, "route",
				store.GetObjectKey(ro), "V1A2", isRouteV1A2(ro))

			// route made it this far: attach!
			ret = append(ret, ro)
		}
	}

	return ret
}

func resolveParentRef(ro *stnrgwv1.UDPRoute, p *gwapiv1.ParentReference, gw *gwapiv1.Gateway, l *gwapiv1.Listener) (bool, string) {
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

	if p.SectionName != nil && *p.SectionName != l.Name {
		return false, fmt.Sprintf("parent SectionName %q does not match listener name %q",
			string(*p.SectionName), l.Name)
	}

	return true, ""
}

func gatewayAllowsNamespace(ro *stnrgwv1.UDPRoute, gw *gwapiv1.Gateway, l *gwapiv1.Listener) (bool, string) {
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

func gatewayAllowsSameNamespace(ro *stnrgwv1.UDPRoute, gw *gwapiv1.Gateway) (bool, string) {
	allowed := gw.GetNamespace() == ro.GetNamespace()
	if !allowed {
		return false, fmt.Sprintf("parent %q/%q (namespace attachment policy: Same) rejects route %q/%q",
			gw.GetName(), gw.GetNamespace(), ro.GetName(), ro.GetNamespace())
	}
	return true, ""
}

func initRouteStatus(ro *stnrgwv1.UDPRoute) {
	ro.Status.Parents = []gwapiv1.RouteParentStatus{}
}

// isParentController returns true if at least one of the parents of the route is controlled by us
func (r *renderer) isRouteControlled(ro *stnrgwv1.UDPRoute) bool {
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
func (r *renderer) isParentOutContext(gws *store.GatewayStore, ro *stnrgwv1.UDPRoute, p *gwapiv1.ParentReference) bool {
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
func (r *renderer) isParentAcceptingRoute(ro *stnrgwv1.UDPRoute, p *gwapiv1.ParentReference, className string) (bool, bool) {
	// r.log.V(4).Info("isParentAcceptingRoute", "route", store.GetObjectKey(ro),
	// 	"parent", store.DumpParentRef(p))

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

func (r *renderer) getParentGateway(ro *stnrgwv1.UDPRoute, p *gwapiv1.ParentReference) *gwapiv1.Gateway {
	// find the corresponding gateway
	ns := ro.GetNamespace()
	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: string(p.Name)}
	return store.Gateways.GetObject(namespacedName)
}

// invalidateMaskedRoutes invalidates a masked GWAPIV1A2 UDPROute
func (r *renderer) invalidateMaskedRoutes(c *RenderContext) {
	for _, ro := range store.UDPRoutesV1A2.GetAll() {
		if !isRouteMasked(ro) || !r.isRouteControlled(ro) {
			continue
		}

		initRouteStatus(ro)
		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]
			parentExists, parentAccept := r.isParentAcceptingRoute(ro, &p, "")
			// automatically handles masked routes
			setRouteConditionStatus(ro, &p, config.ControllerName, parentExists, parentAccept, nil)
		}

		c.update.UpsertQueue.UDPRoutesV1A2.Upsert(ro)
	}
}

func setRouteConditionStatus(ro *stnrgwv1.UDPRoute, p *gwapiv1.ParentReference, controllerName string, exists, accepted bool, backendErr error) {
	// ns := gwapiv1.Namespace(ro.GetNamespace())
	// gr := gwapiv1.Group(gwapiv1.GroupVersion.Group)
	// kind := gwapiv1.Kind("Gateway")

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
			metav1.ConditionFalse, "GwAPI.v1 UDPRoute masked by a STUNnerV1 UDPRoute")
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
		switch {
		case IsNonCriticalError(backendErr, InvalidBackendKind), IsNonCriticalError(backendErr, InvalidBackendGroup):
			// "RouteReasonInvalidKind" is used with the "ResolvedRefs" condition when
			// one of the Route's rules has a reference to an unknown or unsupported
			// Group and/or Kind.
			reason = gwapiv1.RouteReasonInvalidKind
		default:
			reason = gwapiv1.RouteReasonBackendNotFound
		}
		resolvedCond = metav1.Condition{
			Type:               string(gwapiv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: ro.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(reason),
			Message:            "at least one backend reference failed to be successfully resolved",
		}
	} else {
		resolvedCond = metav1.Condition{
			Type:               string(gwapiv1.RouteConditionResolvedRefs),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: ro.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.RouteReasonResolvedRefs),
			Message:            "all backend references successfully resolved",
		}
	}

	meta.SetStatusCondition(&s.Conditions, resolvedCond)

	ro.Status.Parents = append(ro.Status.Parents, s)
}

func setRouteAcceptedCondition(ro *stnrgwv1.UDPRoute, s *[]metav1.Condition, reason gwapiv1.RouteConditionReason, status metav1.ConditionStatus, message string) {
	meta.SetStatusCondition(s, metav1.Condition{
		Type:               string(gwapiv1.RouteConditionAccepted),
		Status:             status,
		ObservedGeneration: ro.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(reason),
		Message:            message,
	})
}

// check by pointer: namespacedname is not unique across stunnerv1 and v1a2 routes
//
//nolint:unused
func isRouteV1(ro client.Object) bool {
	return store.UDPRoutes.Get(store.GetNamespacedName(ro)) == ro
}

// check by pointer: namespacedname is not unique across stunnerv1 and v1a2 routes
func isRouteV1A2(ro client.Object) bool {
	return store.UDPRoutesV1A2.Get(store.GetNamespacedName(ro)) == ro
}

func isRouteMasked(ro client.Object) bool {
	return isRouteV1A2(ro) && store.UDPRoutes.Get(store.GetNamespacedName(ro)) != nil
}
