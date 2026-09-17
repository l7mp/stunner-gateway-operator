package renderer

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

var _ clusterRenderer = &defaultClusterRenderer{}

type defaultClusterRenderer struct{ log logr.Logger }

func newClusterRenderer(log logr.Logger) clusterRenderer {
	return &defaultClusterRenderer{log: log}
}

// renderCluster renders a stunnerd cluster config for a route. The route kind determines the
// cluster protocol and the rendering strategy. Rendering a TCPRoute into a TCP cluster is a
// premium feature: here it fails non-critically, so that the route is still accepted and the
// reason surfaces on its ResolvedRefs condition.
func (r *defaultClusterRenderer) renderCluster(ro store.Route) (*stnrapiv1.ClusterConfig, error) {
	switch ro := ro.(type) {
	case *stnrgwv1.UDPRoute:
		return r.renderServiceCluster(ro, ro.Spec.Rules, stnrapiv1.ClusterProtocolUDP)
	case *stnrgwv1.TCPRoute:
		return nil, NewNonCriticalError(FeatureNotLicensed)
	default:
		return nil, NewCriticalError(InternalError)
	}
}

// renderServiceCluster renders a cluster config from a set of route rules whose backend
// references resolve to Kubernetes Services or StaticServices.
func (r *defaultClusterRenderer) renderServiceCluster(ro store.Route, rs []stnrgwv1.RouteRule, proto stnrapiv1.ClusterProtocol) (*stnrapiv1.ClusterConfig, error) {
	// track down the backendref
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

	ctype, prevCType := stnrapiv1.ClusterTypeStatic, stnrapiv1.ClusterTypeUnknown
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

		if prevCType != stnrapiv1.ClusterTypeUnknown && prevCType != ctype {
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

	if ctype == stnrapiv1.ClusterTypeUnknown {
		return nil, NewNonCriticalError(BackendNotFound)
	}

	cluster := stnrapiv1.ClusterConfig{
		Name:      store.GetObjectKey(ro),
		Type:      ctype.String(),
		Protocol:  proto.String(),
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

func getEndpointsForService(b *stnrgwv1.BackendRef, ns string) ([]string, stnrapiv1.ClusterType, error) {
	ctype := stnrapiv1.ClusterTypeUnknown
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

	ctype = stnrapiv1.ClusterTypeStatic
	ep = append(ep, ips...)

	return ep, ctype, nil
}

// either the ClusterIP if EDS is enabled, or a STRICT_DNS route if EDS is disabled
func getClusterRouteForService(b *stnrgwv1.BackendRef, ns string) ([]string, stnrapiv1.ClusterType, error) {
	var ctype stnrapiv1.ClusterType
	ep := []string{}

	if config.EnableEndpointDiscovery {
		ctype = stnrapiv1.ClusterTypeStatic
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
		ctype = stnrapiv1.ClusterTypeStrictDNS
		ep = append(ep, fmt.Sprintf("%s.%s.svc.cluster.local", string(b.Name), ns))
	}

	return ep, ctype, nil
}

func getEndpointsForStaticService(b *stnrgwv1.BackendRef, ns string) ([]string, stnrapiv1.ClusterType, error) {
	ctype := stnrapiv1.ClusterTypeUnknown
	ep := []string{}

	n := types.NamespacedName{Namespace: ns, Name: string(b.Name)}
	ssvc := store.StaticServices.GetObject(n)
	if ssvc == nil {
		return ep, ctype, NewNonCriticalError(BackendNotFound)
	}

	// ignore Spec.Ports
	ep = make([]string, len(ssvc.Spec.Prefixes))
	copy(ep, ssvc.Spec.Prefixes)

	return ep, stnrapiv1.ClusterTypeStatic, nil
}

func injectPortRange(b *stnrgwv1.BackendRef, eps []string, ctype stnrapiv1.ClusterType) error {
	// only static clusters know how to handle port ranges
	if ctype != stnrapiv1.ClusterTypeStatic {
		return nil
	}

	port, endPort := stnrapiv1.DefaultMinRelayPort, stnrapiv1.DefaultMaxRelayPort
	if b.Port != nil && int(*b.Port) > 0 && int(*b.Port) < 65536 {
		port = int(*b.Port)
		endPort = int(*b.Port)
	}
	if b.EndPort != nil && int(*b.EndPort) > 0 && int(*b.EndPort) < 65536 && int(*b.EndPort) >= port {
		endPort = int(*b.EndPort)
	}

	// default port range is not injected
	if port != stnrapiv1.DefaultMinRelayPort || endPort != stnrapiv1.DefaultMaxRelayPort {
		for i := range eps {
			eps[i] += fmt.Sprintf(":<%d-%d>", port, endPort)
		}
	}

	return nil
}
