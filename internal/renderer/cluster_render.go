package renderer

import (
	"net"
	"net/netip"
	"regexp"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var _ clusterRenderer = &defaultClusterRenderer{}

// portRangeMatcher matches a StaticService prefix with a port range: "<prefix>:<min-max>".
var portRangeMatcher = regexp.MustCompile("^(.*):<([0-9]+)-([0-9]+)>$")

type defaultClusterRenderer struct{ log logr.Logger }

func newClusterRenderer(log logr.Logger) clusterRenderer {
	return &defaultClusterRenderer{log: log}
}

// clusterName names the cluster of a route: the namespaced name of the route and the protocol of
// its kind, so that the clusters of routes of different kinds never clash.
func clusterName(ro store.Route) string {
	proto := "unknown"
	switch ro.(type) {
	case *stnrgwv1.UDPRoute:
		proto = "udp"
	case *stnrgwv1.TCPRoute:
		proto = "tcp"
	case *stnrgwv1a1.TURNRoute:
		proto = "turn"
	}
	return store.GetObjectKey(ro) + "/" + proto
}

// renderCluster renders a stunnerd cluster config for a route. The route kind determines the
// cluster protocol and the rendering strategy.
// Rendering a TCPRoute into a TCP cluster is a premium feature: here it fails non-critically, so
// that the route is still accepted and the reason surfaces on its ResolvedRefs condition.
func (r *defaultClusterRenderer) renderCluster(_ *RenderContext, ro store.Route) (*stnrapiv2.ClusterConfig, []stnrapiv2.FilterRule, error) {
	switch ro := ro.(type) {
	case *stnrgwv1.UDPRoute:
		return r.renderServiceCluster(ro, ro.Spec.Rules, stnrapiv2.ProtocolUDP)
	case *stnrgwv1.TCPRoute:
		return nil, nil, NewNonCriticalError(FeatureNotLicensed)
	case *stnrgwv1a1.TURNRoute:
		return r.renderTunnelCluster(ro)
	default:
		return nil, nil, NewCriticalError(InternalError)
	}
}

// renderTunnelCluster renders the cluster of a TURNRoute: the cluster of its backends (see
// renderServiceCluster, no backends is no endpoint), tunnelled through the upstream TURN server
// of the route, presenting the authentication policy of the route if it has one. The tunnel
// admits every peer: the filter rules of the route are a single ALLOW of every address.
func (r *defaultClusterRenderer) renderTunnelCluster(ro *stnrgwv1a1.TURNRoute) (*stnrapiv2.ClusterConfig, []stnrapiv2.FilterRule, error) {
	if u, err := stnrapiv2.ParseURI(ro.Spec.Tunnel.URI); err != nil || !u.Protocol.IsTURN() {
		return nil, nil, NewNonCriticalError(InvalidTunnelURI)
	}
	auth, err := renderTunnelAuth(ro)
	if err != nil {
		return nil, nil, err
	}

	var cluster *stnrapiv2.ClusterConfig
	var routeError error
	if len(ro.Spec.Rules) > 0 {
		if cluster, _, routeError = r.renderServiceCluster(ro, ro.Spec.Rules, stnrapiv2.ProtocolUDP); cluster == nil {
			return nil, nil, routeError
		}
	} else {
		cluster = &stnrapiv2.ClusterConfig{
			Name:     clusterName(ro),
			Protocol: stnrapiv2.ProtocolUDP.String(),
			// $STUNNER_ADDR, the relay address, filled in from the pod environment
			Addrs: []string{opdefault.DefaultSTUNnerAddressEnvVarName},
		}
	}
	cluster.Tunnel = &stnrapiv2.TunnelConfig{URL: ro.Spec.Tunnel.URI, Insecure: ro.Spec.Tunnel.Insecure,
		SNI: ro.Spec.Tunnel.SNI}
	if auth != nil {
		cluster.Tunnel.Auth = auth.Name
	}

	// validate so that defaults get filled in
	if err := cluster.Validate(); err != nil {
		return nil, nil, NewNonCriticalError(InvalidTunnelURI)
	}

	r.log.V(2).Info("Finished rendering tunnel cluster config", "route", store.GetObjectKey(ro),
		"result", cluster.String())

	return cluster, []stnrapiv2.FilterRule{{Action: stnrapiv2.FilterActionAllow.String()}}, routeError
}

// renderServiceCluster renders a cluster config from a set of route rules whose backend
// references resolve to Kubernetes Services or StaticServices. A backend address that does not
// parse as an IP address or prefix is skipped.
//
// The endpoints are the single addresses of the backends, at the port of the backend if it sets one:
// the port a server dialing the backend uses, never a port range. Next to the cluster it returns
// the filter rules of the route, for the filter renderer to render into explicit filters: an ALLOW
// rule per address or prefix of the backends and per ClusterIP if relaying to it is enabled (the
// ClusterIP is a peer but no endpoint: kube-proxy balances it already), with the port range of the
// backend (port and endPort) or of a StaticService prefix written as "<prefix>:<min-max>".
func (r *defaultClusterRenderer) renderServiceCluster(ro store.Route, rs []stnrgwv1.RouteRule, proto stnrapiv2.Protocol) (*stnrapiv2.ClusterConfig, []stnrapiv2.FilterRule, error) {

	// track down the backendref
	if len(rs) == 0 {
		return nil, nil, NewCriticalError(NoRuleFound)
	}

	if len(rs) > 1 {
		r.log.V(1).Info("Cluster rendering error: too many rules in route, considering only "+
			"the first one", "route", store.GetObjectKey(ro), "rules", len(rs))
	}

	eps := []string{}
	peers := []stnrapiv2.FilterRule{}

	// the rest of the errors are not critical, but we still need to keep track of each in
	// order to set the ResolvedRefs Route status: last error is reported only
	// the filter ports of a port range; the full range restricts nothing
	filterPorts := func(port, endPort int) []stnrapiv2.FilterPort {
		if port == stnrapiv2.DefaultMinRelayPort && endPort == stnrapiv2.DefaultMaxRelayPort {
			return nil
		}
		p := stnrapiv2.FilterPort{Port: port}
		if endPort != port {
			p.EndPort = endPort
		}
		return []stnrapiv2.FilterPort{p}
	}

	var routeError error
	for _, b := range rs[0].BackendRefs {
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
		n := types.NamespacedName{Namespace: ns, Name: string(b.Name)}

		addrs := []string{}
		clusterIPs := []string{} // admitted as peers but no endpoint: kube-proxy balances them
		switch ref := &b; {
		case store.IsReferenceService(ref):
			epAddrs, errEps := getEndpointAddrs(n, false)
			if errEps != nil {
				r.log.V(1).Info("Cluster rendering error: could not find the EndpointSlices of "+
					"Service backend", "route", store.GetObjectKey(ro), "backendRef",
					store.DumpBackendRef(ref), "error", errEps)
				routeError = errEps
			} else {
				addrs = append(addrs, epAddrs...)
			}

			// the ClusterIP, if relaying to it is enabled
			var errCluster error
			if config.EnableRelayToClusterIP {
				if clusterIPs, errCluster = getClusterIP(n); errCluster != nil {
					r.log.V(1).Info("Cluster rendering error: could not find the ClusterIP "+
						"of Service backend", "route",
						store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(ref),
						"error", errCluster)
					routeError = errCluster
				}
			}

			if errCluster != nil && errEps != nil {
				// both attempts failed: skip backend
				r.log.V(1).Info("Cluster rendering: skipping Service backend", "route",
					store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(ref),
					"reason", routeError)
				routeError = NewNonCriticalError(BackendNotFound)
				continue
			}

		case store.IsReferenceStaticService(ref):
			ssvc := store.StaticServices.GetObject(n)
			if ssvc == nil {
				routeError = NewNonCriticalError(BackendNotFound)
				r.log.Info("Cluster rendering error: could not render endpoints for StaticService backend",
					"route", store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(ref),
					"error", routeError)
				continue
			}
			// ignore Spec.Ports
			addrs = append(addrs, ssvc.Spec.Prefixes...)

		default:
			// error could also be InvalidBackendGroup: both are reported with the same
			// reason in the route status
			routeError = NewNonCriticalError(InvalidBackendKind)
			r.log.Info("Cluster rendering error: invalid backend Kind and/or Group", "route", store.GetObjectKey(ro),
				"backendRef", store.DumpBackendRef(&b), "error", routeError)
			continue
		}

		// the port range of the backend: only a backend setting endPort restricts the ports, a
		// bare port is the mandatory port of a Gateway API backend reference
		var backendPorts []stnrapiv2.FilterPort
		if b.Port != nil && b.EndPort != nil && *b.Port > 0 && *b.Port <= *b.EndPort {
			backendPorts = filterPorts(int(*b.Port), int(*b.EndPort))
		}

		for i, a := range append(addrs, clusterIPs...) {
			// a StaticService prefix may carry its own port range, which overrides the
			// port range of the backend
			ports := backendPorts
			if m := portRangeMatcher.FindStringSubmatch(a); m != nil {
				p, _ := strconv.Atoi(m[2])
				e, _ := strconv.Atoi(m[3])
				if p < 1 || e < p || e > 65535 {
					routeError = NewNonCriticalError(InvalidPortRange)
					r.log.Info("Cluster rendering error: skipping backend address", "route",
						store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b),
						"address", a, "error", routeError)
					continue
				}
				a, ports = m[1], filterPorts(p, e)
			}

			prefix, err := netip.ParsePrefix(a)
			if err != nil {
				addr, aerr := netip.ParseAddr(a)
				if aerr != nil {
					routeError = NewNonCriticalError(InvalidEndpoint)
					r.log.Info("Cluster rendering error: skipping backend address", "route",
						store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b),
						"address", a, "error", routeError)
					continue
				}
				prefix = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
			}
			prefix = prefix.Masked()

			// a prefix and the ClusterIP are peer blocks only, an address is an endpoint too, at
			// the port of the backend if it has one: the port a server dialing the backend uses
			if prefix.IsSingleIP() && i < len(addrs) {
				ep := prefix.Addr().String()
				if b.Port != nil && *b.Port > 0 {
					ep = net.JoinHostPort(ep, strconv.Itoa(int(*b.Port)))
				}
				eps = append(eps, ep)
			}
			peers = append(peers, stnrapiv2.FilterRule{
				Action:  stnrapiv2.FilterActionAllow.String(),
				IPBlock: &stnrapiv2.IPBlock{CIDR: prefix.String()},
				Ports:   ports,
			})
		}

		r.log.V(2).Info("Cluster rendering: adding backend addresses", "route",
			store.GetObjectKey(ro), "backendRef", store.DumpBackendRef(&b), "addresses", addrs)
	}

	cluster := stnrapiv2.ClusterConfig{
		Name:      clusterName(ro),
		Type:      stnrapiv2.ClusterTypeStatic.String(),
		Endpoints: eps,
		Protocol:  proto.String(),
		// $STUNNER_ADDR, the relay address, filled in from the pod environment
		Addrs: []string{opdefault.DefaultSTUNnerAddressEnvVarName},
	}

	// validate so that defaults get filled in
	if err := cluster.Validate(); err != nil {
		return nil, nil, err
	}

	backendStatus := "None"
	if routeError != nil {
		backendStatus = routeError.Error()
	}

	r.log.V(2).Info("Finished rendering cluster config", "route", store.GetObjectKey(ro), "result",
		cluster.String(), "peers", len(peers), "backend-error", backendStatus)

	return &cluster, peers, routeError
}
