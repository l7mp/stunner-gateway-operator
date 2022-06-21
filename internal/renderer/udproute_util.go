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
	r.log.V(4).Info("getUDPRoutes4Listener", "Gateway", store.GetObjectKey(gw), "listener",
		l.Name)

	ret := make([]*gatewayv1alpha2.UDPRoute, 0)
	rs := r.op.GetUDPRoutes()

	for i := range rs {
		ro := rs[i]
		r.log.V(4).Info("getUDPRoutes4Listener: considering route for listener", "Gateway",
			store.GetObjectKey(gw), "listener", l.Name, "route",
			store.GetObjectKey(ro))

		// FromNamespaces("Same")
		if gw.GetNamespace() != ro.GetNamespace() {
			r.log.V(4).Info("getUDPRoutes4Listener: route namespace does not match "+
				"gateway namespace", "Gateway", store.GetObjectKey(gw), "route",
				store.GetObjectKey(ro))
			continue
		}

		for j := range ro.Spec.CommonRouteSpec.ParentRefs {
			p := ro.Spec.CommonRouteSpec.ParentRefs[j]
			if resolveParentRef(&p, gw, l) == false {
				r.log.V(4).Info("getUDPRoutes4Listener: route rejected for listener",
					"Gateway", store.GetObjectKey(gw), "listener", l.Name,
					"route", store.GetObjectKey(ro), "parentRef", p.Name)

				continue
			}

			r.log.V(4).Info("getUDPRoutes4Listener: route found", "Gateway",
				store.GetObjectKey(gw), "listener", l.Name, "route",
				store.GetObjectKey(ro))

			// route made it this far: attach!
			ret = append(ret, ro)
		}

	}

	return ret
}

func resolveParentRef(p *gatewayv1alpha2.ParentRef, gw *gatewayv1alpha2.Gateway, l *gatewayv1alpha2.Listener) bool {
	if p.Group != nil && *p.Group != gatewayv1alpha2.Group(gatewayv1alpha2.GroupVersion.Group) {
		return false
	}
	if p.Kind != nil && *p.Kind != "Gateway" {
		return false
	}
	if p.Namespace != nil && *p.Namespace != gatewayv1alpha2.Namespace(gw.GetNamespace()) {
		return false
	}
	if p.Name != gatewayv1alpha2.ObjectName(gw.GetName()) {
		return false
	}
	if p.SectionName != nil && *p.SectionName != l.Name {
		return false
	}

	return true
}

func initRouteStatus(ro *gatewayv1alpha2.UDPRoute) {
	ro.Status.Parents = []gatewayv1alpha2.RouteParentStatus{}
}

func (r *Renderer) isParentAcceptingRoute(ro *gatewayv1alpha2.UDPRoute, p *gatewayv1alpha2.ParentRef) bool {
	r.log.V(4).Info("isParentAcceptingRoute", "route", store.GetObjectKey(ro),
		"parent", fmt.Sprintf("%#v", *p))

	// find the corresponding gateway
	ns := ro.GetNamespace()
	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	namespacedName := types.NamespacedName{Namespace: ns, Name: string(p.Name)}
	gw := r.op.GetGateway(namespacedName)
	if gw == nil {
		r.log.V(4).Info("isParentAcceptingRoute: no gateway found for ParentRef", "route",
			store.GetObjectKey(ro), "parent", fmt.Sprintf("%#v", *p))
		return false
	}

	// is there a listener that accepts us?
	for i := range gw.Spec.Listeners {
		l := gw.Spec.Listeners[i]

		if resolveParentRef(p, gw, &l) == true {
			r.log.V(4).Info("isParentAcceptingRoute: gateway/listener found for ParentRef",
				"route", store.GetObjectKey(ro), "parent", fmt.Sprintf("%#v", *p),
				"gateway", gw.GetName(), "listener", l.Name)

			return true
		}
	}

	r.log.V(4).Info("isParentAcceptingRoute result", "route", store.GetObjectKey(ro),
		"parent", fmt.Sprintf("%#v", *p), "result", "rejected")

	return false
}

func setRouteConditionStatus(ro *gatewayv1alpha2.UDPRoute, p *gatewayv1alpha2.ParentRef, controllerName string, accepted bool) {
	s := gatewayv1alpha2.RouteParentStatus{
		ParentRef:      *p,
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
		Status:             metav1.ConditionTrue,
		ObservedGeneration: ro.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ResolvedRefs",
		Message:            "parent reference successfully resolved",
	})

	ro.Status.Parents = append(ro.Status.Parents, s)
}
