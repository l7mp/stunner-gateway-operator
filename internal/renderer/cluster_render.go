package renderer

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	// corev1 "k8s.io/api/core/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// FIXME handle endpoint discovery for non-headless services
func (r *Renderer) renderCluster(ro *gatewayv1alpha2.UDPRoute) (*stunnerconfv1alpha1.ClusterConfig, error) {
	r.log.V(4).Info("renderCluster", "route", store.GetObjectKey(ro))

	// track down the backendref
	rs := ro.Spec.Rules
	if len(rs) == 0 {
		return nil, fmt.Errorf("no rules found in route %q", store.GetObjectKey(ro))
	}

	if len(rs) > 1 {
		r.log.V(1).Info("renderCluster: too many rules (%d) in route %q, "+
			"considering only the first one", len(rs), store.GetObjectKey(ro))
	}

	eps := []string{}

	ctype := stunnerconfv1alpha1.ClusterTypeStatic
	for _, b := range rs[0].BackendRefs {

		// core.v1 has empty Group
		if b.Group != nil && *b.Group != gatewayv1alpha2.Group("") {
			r.log.V(2).Info("renderCluster: invalid Group in backend reference (ignoring)",
				"route", store.GetObjectKey(ro), "group", *b.Group)
			continue
		}

		if b.Kind != nil && *b.Kind != "Service" {
			r.log.V(2).Info("renderCluster: invalid Kind in backend reference, epecting Service (ignoring)",
				"route", store.GetObjectKey(ro), "kind", *b.Kind)
			continue
		}

		// default is the local namespace of the route
		ns := ro.GetNamespace()
		if b.Namespace != nil {
			ns = string(*b.Namespace)
		}

		ep := []string{}
		if config.EnableEndpointDiscovery || config.EnableRelayToClusterIP {
			ctype = stunnerconfv1alpha1.ClusterTypeStatic
			n := types.NamespacedName{
				Namespace: ns,
				Name:      string(b.Name),
			}

			if config.EnableEndpointDiscovery {
				ep = append(ep, getEndpointAddrs(n, false)...)
			}

			if config.EnableRelayToClusterIP {
				ep = append(ep, getClusterIP(n)...)
			}
		} else {
			// fall back to strict DNS and hope for the best
			ctype = stunnerconfv1alpha1.ClusterTypeStrictDNS
			ep = append(ep, fmt.Sprintf("%s.%s.svc.cluster.local", string(b.Name), ns))
		}

		r.log.V(3).Info("renderCluster: adding Endpoints to endpoint list", "route",
			store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b), "cluster-type",
			ctype.String(), "endpoints", ep)

		eps = append(eps, ep...)
	}

	cluster := stunnerconfv1alpha1.ClusterConfig{
		Name:      store.GetObjectKey(ro),
		Type:      ctype.String(),
		Endpoints: eps,
	}

	// validate so that defaults get filled in
	if err := cluster.Validate(); err != nil {
		return nil, err
	}

	r.log.V(2).Info("renderCluster ready", "route", store.GetObjectKey(ro), "result",
		fmt.Sprintf("%#v", cluster))

	return &cluster, nil
}
