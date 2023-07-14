package renderer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) renderCluster(ro *gwapiv1a2.UDPRoute) (*stnrconfv1a1.ClusterConfig, error) {
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
	var backendErr error

	ctype, prevCType := stnrconfv1a1.ClusterTypeStatic, stnrconfv1a1.ClusterTypeUnknown
	for _, b := range rs[0].BackendRefs {
		b := b

		if b.Group != nil && string(*b.Group) != corev1.GroupName &&
			string(*b.Group) != stnrv1a1.GroupVersion.Group {
			backendErr = NewNonCriticalError(InvalidBackendGroup)
			r.log.V(2).Info("renderCluster: invalid backend Group", "route",
				store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b), "group",
				*b.Group, "error", backendErr.Error())
			continue
		}

		if b.Kind != nil && string(*b.Kind) != "Service" && string(*b.Kind) != "StaticService" {
			backendErr = NewNonCriticalError(InvalidBackendKind)
			r.log.V(2).Info("renderCluster: invalid backend Kind", "route",
				store.GetObjectKey(ro), "backendRef", dumpBackendRef(&b), "kind", *b.Kind,
				"error", backendErr)
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
			var err error

			// get endpoints if EDS is enabled
			if config.EnableEndpointDiscovery {
				epEDS, ctypeEDS, errEDS := getEndpointsForService(ref, ns)
				if errEDS != nil {
					backendErr = errEDS
					r.log.V(2).Info("renderCluster: error rendering Endpoints for Service backend",
						"route", store.GetObjectKey(ro), "backendRef", dumpBackendRef(ref),
						"error", backendErr)
				}
				eps = append(eps, epEDS...)
				ctype = ctypeEDS
				err = errEDS
			}

			// the clusterIP or STRICT_DNS cluster if EDS is disabled
			epCluster, ctypeCluster, errCluster := getClusterRouteForService(ref, ns)
			if errCluster != nil {
				backendErr = errCluster
				r.log.V(2).Info("renderCluster: error rendering route for Service backend",
					"route", store.GetObjectKey(ro), "backendRef", dumpBackendRef(ref),
					"error", backendErr)
			}

			if errCluster != nil && err != nil {
				// both attempts failed: skip backend
				continue
			}

			eps = append(eps, epCluster...)
			ctype = ctypeCluster

		case store.IsReferenceStaticService(ref):
			var err error
			ep, ctype, err = getEndpointsForStaticService(ref, ns)
			if err != nil {
				backendErr = err
				r.log.V(2).Info("renderCluster: error rendering endpoints for StaticService backend",
					"route", store.GetObjectKey(ro), "backendRef", dumpBackendRef(ref),
					"error", backendErr)
				continue
			}
		default:
			// error could also be InvalidBackendGroup: both are reported with the same
			// reason in the route status
			backendErr = NewNonCriticalError(InvalidBackendKind)
			r.log.Info("renderCluster: invalid backend Kind and/or Group", "route", store.GetObjectKey(ro),
				"backendRef", dumpBackendRef(&b), "error", backendErr)
			continue
		}

		if prevCType != stnrconfv1a1.ClusterTypeUnknown && prevCType != ctype {
			backendErr = NewNonCriticalError(InconsitentClusterType)
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

	if ctype == stnrconfv1a1.ClusterTypeUnknown {
		return nil, NewNonCriticalError(BackendNotFound)
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

func getEndpointsForService(b *gwapiv1a2.BackendRef, ns string) ([]string, stnrconfv1a1.ClusterType, error) {
	ctype := stnrconfv1a1.ClusterTypeUnknown
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

	ctype = stnrconfv1a1.ClusterTypeStatic
	ep = append(ep, ips...)

	return ep, ctype, nil
}

// either the ClusterIP if EDS is enabled, or a STRICT_DNS route if EDS is disabled
func getClusterRouteForService(b *gwapiv1a2.BackendRef, ns string) ([]string, stnrconfv1a1.ClusterType, error) {
	var ctype stnrconfv1a1.ClusterType
	ep := []string{}

	if config.EnableEndpointDiscovery {
		ctype = stnrconfv1a1.ClusterTypeStatic
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
		ctype = stnrconfv1a1.ClusterTypeStrictDNS
		ep = append(ep, fmt.Sprintf("%s.%s.svc.cluster.local", string(b.Name), ns))
	}

	return ep, ctype, nil
}

func getEndpointsForStaticService(b *gwapiv1a2.BackendRef, ns string) ([]string, stnrconfv1a1.ClusterType, error) {
	ctype := stnrconfv1a1.ClusterTypeUnknown
	ep := []string{}

	n := types.NamespacedName{Namespace: ns, Name: string(b.Name)}
	ssvc := store.StaticServices.GetObject(n)
	if ssvc == nil {
		return ep, ctype, NewNonCriticalError(BackendNotFound)
	}

	// ignore Spec.Ports
	ep = make([]string, len(ssvc.Spec.Prefixes))
	copy(ep, ssvc.Spec.Prefixes)

	return ep, stnrconfv1a1.ClusterTypeStatic, nil
}
