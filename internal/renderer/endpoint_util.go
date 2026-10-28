package renderer

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
)

// getEndpointAddrs finds the endpoint IP addresses of a Service from its EndpointSlices, ready or
// not, the addresses to dial and, when port is a port of the Service, the target port of each
// address. The addresses to dial are the ready ones or, when none is ready, the serving ones of
// the terminating pods, as kube-proxy picks the endpoints of new connections.
func getEndpointAddrs(n types.NamespacedName, port *corev1.ServicePort) ([]string, map[string]bool, map[string]int, error) {
	ret, targetPorts := []string{}, map[string]int{}
	ready, serving := map[string]bool{}, map[string]bool{}

	// find all endpointslices in the given namespace labeled with the service name
	for _, epsl := range store.EndpointSlices.GetAll() {
		if epsl.GetNamespace() != n.Namespace {
			continue
		}

		svcName, ok := epsl.GetLabels()[discoveryv1.LabelServiceName]
		if !ok || svcName != n.Name {
			continue
		}

		// the target port of the Service port, named as the Service port in every slice
		targetPort := 0
		for _, p := range epsl.Ports {
			if port != nil && p.Port != nil && p.Protocol != nil && *p.Protocol == port.Protocol &&
				(p.Name == nil && port.Name == "" || p.Name != nil && *p.Name == port.Name) {
				targetPort = int(*p.Port)
			}
		}

		for _, ep := range epsl.Endpoints {
			// an unset condition is true, an unset serving condition is the ready one, see
			// https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/#conditions
			isReady := ep.Conditions.Ready == nil || *ep.Conditions.Ready
			isServing := isReady
			if ep.Conditions.Serving != nil {
				isServing = *ep.Conditions.Serving
			}
			for _, a := range ep.Addresses {
				ret = append(ret, a)
				ready[a], serving[a] = isReady, isServing
				if targetPort > 0 {
					targetPorts[a] = targetPort
				}
			}
		}
	}

	if len(ret) == 0 {
		return ret, nil, targetPorts, NewNonCriticalError(EndpointNotFound)
	}

	dial := map[string]bool{}
	for a, ok := range ready {
		if ok {
			dial[a] = true
		}
	}
	if len(dial) == 0 {
		dial = serving
	}
	return ret, dial, targetPorts, nil
}
