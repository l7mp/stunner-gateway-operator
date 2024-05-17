package renderer

import (
	// "fmt"

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// find the list of endpoint IP addresses associated with a service
func getEndpointAddrs(n types.NamespacedName, suppressNotReady bool) ([]string, error) {
	if config.EndpointSliceAvailable {
		return getEndpointAddrsFromEndpointSlice(n, suppressNotReady)
	} else {
		return getEndpointAddrsFromEndpoints(n, suppressNotReady)
	}
}

func getEndpointAddrsFromEndpointSlice(n types.NamespacedName, suppressNotReady bool) ([]string, error) {
	ret := []string{}

	// find all endpointslices in the given namespace labeled with the service name
	for _, epsl := range store.EndpointSlices.GetAll() {
		if epsl.GetNamespace() != n.Namespace {
			continue
		}

		svcName, ok := epsl.GetLabels()[discoveryv1.LabelServiceName]
		if !ok || svcName != n.Name {
			continue
		}

		// process EndpointSlice (ignore EndpointPort)
		for _, ep := range epsl.Endpoints {
			if len(ep.Addresses) == 0 {
				continue
			}

			// consider "serving" pods "ready", see
			// https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/#serving
			ready := ep.Conditions.Ready == nil || *ep.Conditions.Ready
			serving := ep.Conditions.Serving == nil || *ep.Conditions.Serving
			ready = serving || ready
			// terminating := ep.Conditions.Terminating != nil && *ep.Conditions.Terminating

			if suppressNotReady && !ready {
				continue
			}

			ret = append(ret, ep.Addresses...)
		}
	}

	if len(ret) == 0 {
		return ret, NewNonCriticalError(EndpointNotFound)
	}

	return ret, nil
}

func getEndpointAddrsFromEndpoints(n types.NamespacedName, suppressNotReady bool) ([]string, error) {
	ret := []string{}

	ep := store.Endpoints.GetObject(n)
	if ep == nil {
		return ret, NewNonCriticalError(EndpointNotFound)
	}

	// allow clients to reach not-ready addresses: they have already gone through ICE
	// negotiation so they may have a better idea on endpoint-readiness than Kubernetes
	for _, s := range ep.Subsets {
		for _, a := range s.Addresses {
			if a.IP != "" {
				ret = append(ret, a.IP)
			}
		}
		if !suppressNotReady {
			for _, a := range s.NotReadyAddresses {
				if a.IP != "" {
					ret = append(ret, a.IP)
				}
			}
		}
	}

	return ret, nil
}
