package renderer

import (
	// "fmt"
	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
)

type routeGatewayPair struct {
	route    *gatewayv1alpha2.UDPRoute
	gateway  *gatewayv1alpha2.Gateway
	listener *gatewayv1alpha2.Listener
}

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
				r.log.V(4).Info("getUDPRoutes4Listener: route rejected", "Gateway",
					store.GetObjectKey(gw), "listener", l.Name, "route",
					store.GetObjectKey(ro), "parentRef", p.Name)

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

// func setRouteStatusAccepted {

// }
