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
	"k8s.io/apimachinery/pkg/types"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
)

// we implement the below AllowedRoutes policy:
// AllowedRoutes{
// 	Namespaces: &RouteNamespaces{{
// 		From: &FromNamespaces("Same")
// 		Selector: nil
// 	}},
// 	Kinds: []RouteGroupKind{{
// 		Group: Group("gateway.networking.k8s.io"),
// 		Kind:  Kind("UDPRoute"),
// 	}}
// }
func (r *Renderer) getUDPRoutes4Listener(gw *gatewayv1alpha2.Gateway, l *gatewayv1alpha2.Listener) []*gatewayv1alpha2.UDPRoute {
	r.log.V(4).Info("getUDPRoutes4Listener", "gateway", store.GetObjectKey(gw), "listener",
		l.Name)

	ret := make([]*gatewayv1alpha2.UDPRoute, 0)
	rs := store.UDPRoutes.GetAll()

	for i := range rs {
		ro := rs[i]
		r.log.V(4).Info("getUDPRoutes4Listener: considering route for listener", "gateway",
			store.GetObjectKey(gw), "listener", l.Name, "route",
			store.GetObjectKey(ro))

		// FromNamespaces("Same")
		if gw.GetNamespace() != ro.GetNamespace() {
			r.log.V(4).Info("getUDPRoutes4Listener: route namespace does not match "+
				"gateway namespace", "gateway", store.GetObjectKey(gw), "route",
				store.GetObjectKey(ro))
			continue
		}

		for j := range ro.Spec.CommonRouteSpec.ParentRefs {
			p := ro.Spec.CommonRouteSpec.ParentRefs[j]

			found, reason := resolveParentRef(&p, gw, l)
			if found == false {
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

func resolveParentRef(p *gatewayv1alpha2.ParentRef, gw *gatewayv1alpha2.Gateway, l *gatewayv1alpha2.Listener) (bool, string) {
	if p.Group != nil && *p.Group != gatewayv1alpha2.Group(gatewayv1alpha2.GroupVersion.Group) {
		return false, fmt.Sprintf("parent group %q does not match gateway group %q",
			string(*p.Group), gatewayv1alpha2.GroupVersion.Group)
	}
	if p.Kind != nil && *p.Kind != "Gateway" {
		return false, fmt.Sprintf("parent kind %q does not match gateway kind %q",
			string(*p.Kind), "Gateway")
	}
	if p.Namespace != nil && *p.Namespace != gatewayv1alpha2.Namespace(gw.GetNamespace()) {
		return false, fmt.Sprintf("parent namespace %q does not match gateway namespace %q",
			string(*p.Namespace), gw.GetNamespace())
	}
	if p.Name != gatewayv1alpha2.ObjectName(gw.GetName()) {
		return false, fmt.Sprintf("parent name %q does not match gateway name %q",
			string(p.Name), gw.GetName())
	}
	if p.SectionName != nil && *p.SectionName != l.Name {
		return false, fmt.Sprintf("parent SectionName %q does not match listener name %q",
			string(*p.SectionName), l.Name)
	}

	return true, ""
}

func initRouteStatus(ro *gatewayv1alpha2.UDPRoute) {
	ro.Status.Parents = []gatewayv1alpha2.RouteParentStatus{}
}

func (r *Renderer) isParentAcceptingRoute(ro *gatewayv1alpha2.UDPRoute, p *gatewayv1alpha2.ParentRef, className string) bool {
	r.log.V(4).Info("isParentAcceptingRoute", "route", store.GetObjectKey(ro),
		"parent", dumpParentRef(p))

	// find the corresponding gateway
	ns := ro.GetNamespace()
	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: string(p.Name)}
	gw := store.Gateways.GetObject(namespacedName)
	if gw == nil {
		r.log.V(4).Info("no gateway found for Parent", "route",
			store.GetObjectKey(ro), "parent", dumpParentRef(p))
		return false
	}

	// does the parent belong to the class we are processing: we don't want to generate routes
	// for gateways that link to other classes
	if gw.Spec.GatewayClassName != gatewayv1alpha2.ObjectName(className) {
		r.log.V(4).Info("route links to a gateway that is being managed by another "+
			"gateway-class: rejecting", "route", store.GetObjectKey(ro), "parent",
			fmt.Sprintf("%#v", *p), "linked-gateway-class", gw.Spec.GatewayClassName,
			"current-gateway-class", className)
		return false
	}

	// is there a listener that accepts us?
	for i := range gw.Spec.Listeners {
		l := gw.Spec.Listeners[i]

		found, _ := resolveParentRef(p, gw, &l)
		if found == true {
			r.log.V(4).Info("isParentAcceptingRoute: gateway/listener found for parent",
				"route", store.GetObjectKey(ro), "parent", dumpParentRef(p),
				"gateway", gw.GetName(), "listener", l.Name)

			return true
		}
	}

	r.log.V(4).Info("isParentAcceptingRoute result", "route", store.GetObjectKey(ro),
		"parent", fmt.Sprintf("%#v", *p), "result", "rejected")

	return false
}

func setRouteConditionStatus(ro *gatewayv1alpha2.UDPRoute, p *gatewayv1alpha2.ParentRef, controllerName string, accepted bool) {
	// ns := gatewayv1alpha2.Namespace(ro.GetNamespace())
	// gr := gatewayv1alpha2.Group(gatewayv1alpha2.GroupVersion.Group)
	// kind := gatewayv1alpha2.Kind("Gateway")

	pRef := gatewayv1alpha2.ParentRef{
		Name: p.Name,
	}

	if p.Group != nil && *p.Group != gatewayv1alpha2.Group(gatewayv1alpha2.GroupVersion.Group) {
		pRef.Group = p.Group
	}

	if p.Kind != nil && *p.Kind != "Gateway" {
		pRef.Kind = p.Kind
	}

	if p.Namespace != nil && *p.Namespace != gatewayv1alpha2.Namespace(ro.GetNamespace()) {
		pRef.Namespace = p.Namespace
	}

	if p.SectionName != nil {
		pRef.SectionName = p.SectionName
	}

	s := gatewayv1alpha2.RouteParentStatus{
		ParentRef:      pRef,
		ControllerName: gatewayv1alpha2.GatewayController(controllerName),
		Conditions:     []metav1.Condition{},
	}

	c := metav1.ConditionTrue
	reason := "Accepted"
	if accepted == false {
		c = metav1.ConditionFalse
		reason = "NoMatchingListenerHostname"
	}

	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.ConditionRouteAccepted),
		Status:             c,
		ObservedGeneration: ro.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            "parent accepts the route",
	})

	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.ConditionRouteResolvedRefs),
		Status:             c,
		ObservedGeneration: ro.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ResolvedRefs",
		Message:            "parent reference successfully resolved",
	})

	ro.Status.Parents = append(ro.Status.Parents, s)
}

func dumpParentRef(p *gatewayv1alpha2.ParentRef) string {
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

func dumpBackendRef(b *gatewayv1alpha2.BackendRef) string {
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
