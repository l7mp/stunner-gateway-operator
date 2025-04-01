package renderer

import (
	// "fmt"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// find the first node that has a non-empty extenral address in the status and return it; this is
// purely on a best-effort basis: we require LoadBalancer services to be supported for STUNner
// (NodePorts might not work anyway, e.g., on private vpcs)
func getFirstNodeAddr() string {
	for _, n := range store.Nodes.GetAll() {
		if a := store.GetExternalAddress(n); a != "" {
			return a
		}
	}

	return ""
}
