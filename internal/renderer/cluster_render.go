package renderer

import (
	"fmt"

	// corev1 "k8s.io/api/core/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

// FIXME handle endpoint discovery for non-headless services
func (r *Renderer) renderCluster(ro *gatewayv1alpha2.UDPRoute) (*stunnerconfv1alpha1.ClusterConfig, error) {
	r.log.V(4).Info("renderCluster", "route", store.GetObjectKey(ro))

	// track down the backendref and turn it into a FQDN for STUNner strict-dns clusters
	rs := ro.Spec.Rules
	if len(rs) == 0 {
		return nil, fmt.Errorf("no rules found in route %q", store.GetObjectKey(ro))
	}

	if len(rs) > 1 {
		r.log.V(1).Info("renderCluster: too many rules (%d) in route %q, "+
			"considering only the first one", len(rs), store.GetObjectKey(ro))
	}

	fqdns := []string{}
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

		fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", string(b.Name), ns)

		r.log.V(3).Info("renderCluster: adding FQDN to endpoint list",
			"route", store.GetObjectKey(ro), "fqdn", fqdn)

		fqdns = append(fqdns, fqdn)
	}

	ctype := stunnerconfv1alpha1.ClusterTypeStrictDNS
	cluster := stunnerconfv1alpha1.ClusterConfig{
		Name:      ro.Name,
		Type:      ctype.String(),
		Endpoints: fqdns,
	}

	// validate so that defaults get filled in
	if err := cluster.Validate(); err != nil {
		return nil, err
	}

	r.log.V(2).Info("renderCluster ready", "route", store.GetObjectKey(ro), "result",
		fmt.Sprintf("%#v", cluster))

	return &cluster, nil
}
