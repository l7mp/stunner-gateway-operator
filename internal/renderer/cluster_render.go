package renderer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func (r *renderer) renderCluster(ro *stnrgwv1.UDPRoute) (*stnrconfv1.ClusterConfig, error) {
	// r.log.V(4).Info("renderCluster", "route", store.GetObjectKey(ro))

	// track down the backendref
	rs := ro.Spec.Rules
	if len(rs) == 0 {
		return nil, NewCriticalError(NoRuleFound)
	}

	if len(rs) > 1 {
		r.log.V(1).Info("Cluster rendering error: too many rules (%d) in route %q, "+
			"considering only the first one", len(rs), store.GetObjectKey(ro))
	}

	eps := []string{}

	// the rest of the errors are not critical, but we still need to keep track of each in
	// order to set the ResolvedRefs Route status: last error is reported only
	var routeError error

	ctype, prevCType := stnrconfv1.ClusterTypeStatic, stnrconfv1.ClusterTypeUnknown
	for _, b := range rs[0].BackendRefs {
		b := b

		if b.Group != nil && string(*b.Group) != corev1.GroupName &&
			string(*b.Group) != stnrgwv1.GroupVersion.Group {
			routeError = NewNonCriticalError(InvalidBackendGroup)
			r.log.V(2).Info("Cluster rendering error: invalid backend Group", "route",
				store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b), "group",
				*b.Group, "error", routeError.Error())
			continue
		}

		if b.Kind != nil && string(*b.Kind) != "Service" && string(*b.Kind) != "StaticService" {
			routeError = NewNonCriticalError(InvalidBackendKind)
			r.log.V(2).Info("Cluster rendering error: invalid backend Kind", "route",
				store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b), "kind", *b.Kind,
				"error", routeError)
			continue
		}

		// default is the local namespace of the route
		ns := ro.GetNamespace()
		if b.Namespace != nil {
			ns = string(*b.Namespace)
		}

		ep := []string{}
		switch ref := &b; {
		case store.IsReferenceService(ref):
			var errEDS error

			// get endpoints (checks EDS inline)
			if config.EnableEndpointDiscovery {
				epEDS, ctypeEDS, err := getEndpointsForService(ref, ns)
				if err != nil {
					r.log.V(1).Info("Cluster rendering error: could not render Endpoints for Service backend",
						"route", store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(ref),
						"error", err)
					errEDS = err
					routeError = err
				} else {
					ep = append(ep, epEDS...)
					ctype = ctypeEDS
				}
			}

			// the clusterIP or STRICT_DNS cluster if EDS is disabled
			epCluster, ctypeCluster, errCluster := getClusterRouteForService(ref, ns)
			if errCluster != nil {
				r.log.V(1).Info("Cluster rendering error: could not render service-route (ClusterIP/DNS "+
					"route) for Service backend", "route",
					store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(ref),
					"error", errCluster)

				routeError = errCluster
			} else {
				ep = append(ep, epCluster...)
				ctype = ctypeCluster
			}

			if errCluster != nil && errEDS != nil {
				// both attempts failed: skip backend
				r.log.V(1).Info("Cluster rendering: skipping Service backend", "route",
					store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(ref),
					"reason", routeError)
				routeError = NewNonCriticalError(BackendNotFound)
				continue
			}

		case store.IsReferenceStaticService(ref):
			var err error
			ep, ctype, err = getEndpointsForStaticService(ref, ns)
			if err != nil {
				routeError = err
				r.log.Info("Cluster rendering error: could not render endpoints for StaticService backend",
					"route", store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(ref),
					"error", routeError)
				continue
			}
		default:
			// error could also be InvalidBackendGroup: both are reported with the same
			// reason in the route status
			routeError = NewNonCriticalError(InvalidBackendKind)
			r.log.Info("Cluster rendering error: invalid backend Kind and/or Group", "route", store.GetObjectKey(ro),
				"backendRef", store.DumpBackendRef(&b), "error", routeError)
			continue
		}

		if IsNonCriticalError(routeError, BackendNotFound) {
			r.log.Info("Cluster rendering: skipping backend", "route", store.GetObjectKey(ro),
				"backendRef", store.DumpBackendRef(&b))
			continue
		}

		if prevCType != stnrconfv1.ClusterTypeUnknown && prevCType != ctype {
			routeError = NewNonCriticalError(InconsitentClusterType)
			r.log.Info("Cluster rendering error: inconsistent cluster type", "route",
				store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b),
				"prevous-ctype", fmt.Sprintf("%#v", prevCType))
			continue
		}

		if err := injectPortRange(&b, ep, ctype); err != nil {
			routeError = NewNonCriticalError(InvalidPortRange)
			r.log.Info("Cluster rendering error", "route",
				store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b),
				"cluster-ctype", ctype.String(), "error", err.Error())
			continue
		}

		r.log.V(2).Info("Cluster rendering: adding Endpoints for backend", "route",
			store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b),
			"cluster-type", ctype.String(), "endpoints", ep)

		eps = append(eps, ep...)
		prevCType = ctype
	}

	if ctype == stnrconfv1.ClusterTypeUnknown {
		return nil, NewNonCriticalError(BackendNotFound)
	}

	cluster := stnrconfv1.ClusterConfig{
		Name:      store.GetObjectKey(ro),
		Type:      ctype.String(),
		Endpoints: eps,
	}

	// validate so that defaults get filled in
	if err := cluster.Validate(); err != nil {
		return nil, err
	}

	backendStatus := "None"
	if routeError != nil {
		backendStatus = routeError.Error()
	}
	r.log.V(2).Info("Finished rendering cluster config", "route", store.GetObjectKey(ro), "result",
		fmt.Sprintf("%#v", cluster), "backend-error", backendStatus)

	return &cluster, routeError
}

func getEndpointsForService(b *stnrgwv1.BackendRef, ns string) ([]string, stnrconfv1.ClusterType, error) {
	ctype := stnrconfv1.ClusterTypeUnknown
	ep := []string{}

	if !config.EnableEndpointDiscovery {
		return ep, ctype, NewCriticalError(InternalError)
	}

	n := types.NamespacedName{
		Namespace: ns,
		Name:      string(b.Name),
	}

	ips, err := getEndpointAddrs(n, false)
	if err != nil {
		return ep, ctype, err
	}

	ctype = stnrconfv1.ClusterTypeStatic
	ep = append(ep, ips...)

	return ep, ctype, nil
}

// either the ClusterIP if EDS is enabled, or a STRICT_DNS route if EDS is disabled
func getClusterRouteForService(b *stnrgwv1.BackendRef, ns string) ([]string, stnrconfv1.ClusterType, error) {
	var ctype stnrconfv1.ClusterType
	ep := []string{}

	if config.EnableEndpointDiscovery {
		ctype = stnrconfv1.ClusterTypeStatic
		if config.EnableRelayToClusterIP {
			n := types.NamespacedName{
				Namespace: ns,
				Name:      string(b.Name),
			}
			ips, err := getClusterIP(n)
			if err != nil {
				return ep, ctype, err
			}
			ep = append(ep, ips...)
		} else {
			//otherwise, return an empy endpoint list: make this explicit
			ep = []string{}
		}
	} else {
		// fall back to strict DNS and hope for the best
		ctype = stnrconfv1.ClusterTypeStrictDNS
		ep = append(ep, fmt.Sprintf("%s.%s.svc.cluster.local", string(b.Name), ns))
	}

	return ep, ctype, nil
}

func getEndpointsForStaticService(b *stnrgwv1.BackendRef, ns string) ([]string, stnrconfv1.ClusterType, error) {
	ctype := stnrconfv1.ClusterTypeUnknown
	ep := []string{}

	n := types.NamespacedName{Namespace: ns, Name: string(b.Name)}
	ssvc := store.StaticServices.GetObject(n)
	if ssvc == nil {
		return ep, ctype, NewNonCriticalError(BackendNotFound)
	}

	// ignore Spec.Ports
	ep = make([]string, len(ssvc.Spec.Prefixes))
	copy(ep, ssvc.Spec.Prefixes)

	return ep, stnrconfv1.ClusterTypeStatic, nil
}

func injectPortRange(b *stnrgwv1.BackendRef, eps []string, ctype stnrconfv1.ClusterType) error {
	// only static clusters know how to handle port ranges
	if ctype != stnrconfv1.ClusterTypeStatic {
		return nil
	}

	port, endPort := stnrconfv1.DefaultMinRelayPort, stnrconfv1.DefaultMaxRelayPort
	if b.Port != nil && int(*b.Port) > 0 && int(*b.Port) < 65536 {
		port = int(*b.Port)
		endPort = int(*b.Port)
	}
	if b.EndPort != nil && int(*b.EndPort) > 0 && int(*b.EndPort) < 65536 && int(*b.EndPort) >= port {
		endPort = int(*b.EndPort)
	}

	// default port range is not injected
	if port != stnrconfv1.DefaultMinRelayPort || endPort != stnrconfv1.DefaultMaxRelayPort {
		for i := range eps {
			eps[i] += fmt.Sprintf(":<%d-%d>", port, endPort)
		}
	}

	return nil
}
