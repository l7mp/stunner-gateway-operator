package renderer

import (
	// "fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// find the list of endpoint IP addresses associated with a service
func getEndpointAddrs(svc *corev1.Service, suppressNotReady bool) []string {
	ret := []string{}

	n := types.NamespacedName{
		Namespace: svc.GetNamespace(),
		Name:      svc.GetName(),
	}

	ep := store.Endpoints.GetObject(n)
	if ep == nil {
		return ret
	}

	// allow clients to reach nonready addresses: they have already gone through ICE
	// negotiation so they may have a better idea on endpoint-readynesss than Kubernetes
	for _, s := range ep.Subsets {
		for _, a := range s.Addresses {
			if a.IP != "" {
				ret = append(ret, a.IP)
			}
		}
		if suppressNotReady != true {
			for _, a := range s.NotReadyAddresses {
				if a.IP != "" {
					ret = append(ret, a.IP)
				}
			}
		}
	}

	return ret
}
