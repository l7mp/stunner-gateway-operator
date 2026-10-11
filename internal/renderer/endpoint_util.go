package renderer

import (
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// getEndpointAddrs finds the endpoint IP addresses of a Service from its EndpointSlices.
func getEndpointAddrs(n types.NamespacedName, suppressNotReady bool) ([]string, error) {
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
