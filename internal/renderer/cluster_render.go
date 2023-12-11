package renderer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func (r *Renderer) renderCluster(ro *gwapiv1a2.UDPRoute) (*stnrconfv1.ClusterConfig, error) {
	r.log.V(4).Info("renderCluster", "route", store.GetObjectKey(ro))

	// track down the backendref
	rs := ro.Spec.Rules
	if len(rs) == 0 {
		return nil, NewCriticalError(NoRuleFound)
	}

	if len(rs) > 1 {
		r.log.V(1).Info("renderCluster: too many rules (%d) in route %q, "+
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
			r.log.V(2).Info("renderCluster: invalid backend Group", "route",
				store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b), "group",
				*b.Group, "error", routeError.Error())
			continue
		}

		if b.Kind != nil && string(*b.Kind) != "Service" && string(*b.Kind) != "StaticService" {
			routeError = NewNonCriticalError(InvalidBackendKind)
			r.log.V(2).Info("renderCluster: invalid backend Kind", "route",
				store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b), "kind", *b.Kind,
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
					r.log.V(1).Info("renderCluster: error rendering Endpoints for Service backend",
						"route", store.GetObjectKey(ro), "backendRef", dumpBackendRef(ref),
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
				r.log.V(1).Info("renderCluster: error rendering service-route (ClusterIP/DNS route) for Service backend",
					"route", store.GetObjectKey(ro), "backendRef", dumpBackendRef(ref),
					"error", errCluster)
				routeError = errCluster
			} else {
				ep = append(ep, epCluster...)
				ctype = ctypeCluster
			}

			if errCluster != nil && errEDS != nil {
				// both attempts failed: skip backend
				r.log.V(1).Info("renderCluster: skipping Service backend", "route",
					store.GetObjectKey(ro), "backendRef", dumpBackendRef(ref),
					"reason", routeError)
				routeError = NewNonCriticalError(BackendNotFound)
				continue
			}

		case store.IsReferenceStaticService(ref):
			var err error
			ep, ctype, err = getEndpointsForStaticService(ref, ns)
			if err != nil {
				routeError = err
				r.log.Info("renderCluster: error rendering endpoints for StaticService backend",
					"route", store.GetObjectKey(ro), "backendRef", dumpBackendRef(ref),
					"error", routeError)
				continue
			}
		default:
			// error could also be InvalidBackendGroup: both are reported with the same
			// reason in the route status
			routeError = NewNonCriticalError(InvalidBackendKind)
			r.log.Info("renderCluster: invalid backend Kind and/or Group", "route", store.GetObjectKey(ro),
				"backendRef", dumpBackendRef(&b), "error", routeError)
			continue
		}

		if IsNonCriticalError(routeError, BackendNotFound) {
			r.log.Info("renderCluster: skipping backend", "route", store.GetObjectKey(ro),
				"backendRef", dumpBackendRef(&b))
			continue
		}

		if prevCType != stnrconfv1.ClusterTypeUnknown && prevCType != ctype {
			routeError = NewNonCriticalError(InconsitentClusterType)
			r.log.Info("renderCluster: inconsistent cluster type", "route",
				store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b),
				"prevous-ctype", fmt.Sprintf("%#v", prevCType))
			continue
		}

		r.log.V(2).Info("renderCluster: adding Endpoints for backend", "route",
			store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b), "cluster-type",
			ctype.String(), "endpoints", ep)

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
	r.log.V(2).Info("renderCluster ready", "route", store.GetObjectKey(ro), "result",
		fmt.Sprintf("%#v", cluster), "backend-error", backendStatus)

	return &cluster, routeError
}

func getEndpointsForService(b *gwapiv1.BackendRef, ns string) ([]string, stnrconfv1.ClusterType, error) {
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
func getClusterRouteForService(b *gwapiv1.BackendRef, ns string) ([]string, stnrconfv1.ClusterType, error) {
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

func getEndpointsForStaticService(b *gwapiv1.BackendRef, ns string) ([]string, stnrconfv1.ClusterType, error) {
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
