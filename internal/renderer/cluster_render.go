package renderer

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	// corev1 "k8s.io/api/core/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// FIXME handle endpoint discovery for non-headless services
func (r *Renderer) renderCluster(ro *gwapiv1a2.UDPRoute) (*stnrconfv1a1.ClusterConfig, error) {
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

	// the rest of the errors are not critical, but we still need to keep track of each in
	// order to set the ResolvedRefs Route status: last error is reported only
	var backendErr error

	ctype := stnrconfv1a1.ClusterTypeStatic
	for _, b := range rs[0].BackendRefs {

		// core.v1 has empty Group
		if b.Group != nil && *b.Group != gwapiv1a2.Group("") {
			backendErr = NewNonCriticalRenderError(InvalidBackendGroup)
			r.log.V(2).Info("renderCluster: error resolving backend", "route",
				store.GetObjectKey(ro), "backend", string(b.Name), "group",
				*b.Group, "error", backendErr.Error())
			continue
		}

		if b.Kind != nil && *b.Kind != "Service" {
			backendErr = NewNonCriticalRenderError(InvalidBackendKind)
			r.log.V(2).Info("renderCluster: error resolving backend", "route",
				store.GetObjectKey(ro), "backend", string(b.Name), "kind", *b.Kind,
				"error", backendErr)
			continue
		}

		// default is the local namespace of the route
		ns := ro.GetNamespace()
		if b.Namespace != nil {
			ns = string(*b.Namespace)
		}

		ep := []string{}
		if config.EnableEndpointDiscovery || config.EnableRelayToClusterIP {
			ctype = stnrconfv1a1.ClusterTypeStatic
			n := types.NamespacedName{
				Namespace: ns,
				Name:      string(b.Name),
			}

			if config.EnableEndpointDiscovery {
				ips, err := getEndpointAddrs(n, false)
				if err != nil {
					r.log.V(1).Info("renderCluster: could not set endpoint addresses for backend",
						"route", store.GetObjectKey(ro), "backend", string(b.Name),
						"error", err.Error())
					backendErr = err
				}
				// ips is empty
				ep = append(ep, ips...)
			}

			if config.EnableRelayToClusterIP {
				ips, err := getClusterIP(n)
				if err != nil {
					r.log.V(1).Info("renderCluster: could not set ClusterIP for backend",
						"route", store.GetObjectKey(ro), "backend", string(b.Name),
						"error", err.Error())
					backendErr = err
				}
				// ips is empty
				ep = append(ep, ips...)
			}
		} else {
			// fall back to strict DNS and hope for the best
			ctype = stnrconfv1a1.ClusterTypeStrictDNS
			ep = append(ep, fmt.Sprintf("%s.%s.svc.cluster.local", string(b.Name), ns))
		}

		r.log.V(3).Info("renderCluster: adding Endpoints to endpoint list", "route",
			store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b), "cluster-type",
			ctype.String(), "endpoints", ep)

		eps = append(eps, ep...)
	}

	cluster := stnrconfv1a1.ClusterConfig{
		Name:      store.GetObjectKey(ro),
		Type:      ctype.String(),
		Endpoints: eps,
	}

	// validate so that defaults get filled in
	if err := cluster.Validate(); err != nil {
		return nil, err
	}

	backendStatus := "None"
	if backendErr != nil {
		backendStatus = backendErr.Error()
	}
	r.log.V(2).Info("renderCluster ready", "route", store.GetObjectKey(ro), "result",
		fmt.Sprintf("%#v", cluster), "backend-error", backendStatus)

	return &cluster, backendErr
}
