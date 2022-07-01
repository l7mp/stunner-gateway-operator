package renderer

import (
	// "fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// find the first node that has a non-empty extenral address in the status and return it; this is
// purely on a best-effort basis: we require LoadBalancer services to be supported for STUNner
// (NodePorts might mot work anyway, e.g., on private vpcs)
func getFirstNodeAddr() string {
	for _, n := range store.Nodes.GetAll() {
		for _, a := range n.Status.Addresses {
			if a.Type == corev1.NodeExternalIP && a.Address != "" {
				return a.Address
			}

		}
	}

	return ""
}
