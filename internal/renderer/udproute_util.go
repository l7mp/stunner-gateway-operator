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
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
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
func (r *Renderer) getUDPRoutes4Gateway(gw *gatewayv1alpha2.Gateway) []*gatewayv1alpha2.UDPRoute {
	ret := make([]*gatewayv1alpha2.UDPRoute, 0)
	rs := r.op.GetUDPRoutes()

	for _, l := range gw.Spec.Listeners {
		sectionName := l.Name
		for _, r := range rs {
			// FromNamespaces("Same")
			if gw.GetNamespace() != r.GetNamespace() {
				continue
			}

			for _, p := range r.Spec.CommonRouteSpec.ParentRefs {
				if p.Group != nil && *p.Group != gatewayv1alpha2.Group(gatewayv1alpha2.GroupVersion.Group) {
					continue
				}
				if p.Kind != nil && *p.Kind != "Gateway" {
					continue
				}
				if p.Namespace != nil && *p.Namespace != gatewayv1alpha2.Namespace(gw.GetNamespace()) {
					continue
				}
				if p.Name != gatewayv1alpha2.ObjectName(gw.GetName()) {
					continue
				}
				if p.SectionName != nil && *p.SectionName != sectionName {
					continue
				}

				// route made it this far: attach!
				ret = append(ret, r)
			}
		}
	}

	return ret
}

func (r *Renderer) removeRouteStatus() {
	for _, r := range r.op.GetUDPRoutes() {
		r.Status.Parents = []gatewayv1alpha2.RouteParentStatus{}
	}
}

// func setRouteStatusAccepted {

// }
