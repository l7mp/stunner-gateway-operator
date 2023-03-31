package renderer

import (
	// "fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// find the list of endpoint IP addresses associated with a service
func getEndpointAddrs(n types.NamespacedName, suppressNotReady bool) ([]string, error) {
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
