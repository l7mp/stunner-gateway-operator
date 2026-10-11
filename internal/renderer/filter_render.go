package renderer

import (
	"slices"

	"github.com/go-logr/logr"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
)

var _ filterRenderer = &defaultFilterRenderer{}

type defaultFilterRenderer struct{ log logr.Logger }

func newFilterRenderer(log logr.Logger) filterRenderer {
	return &defaultFilterRenderer{log: log}
}

// renderClusterFilter renders the port filter of a cluster, named after the cluster: the peers of
// the cluster on the ports of their backends, then a deny of the rest. It returns nil when no
// backend restricts its ports.
func (r *defaultFilterRenderer) renderClusterFilter(cluster *stnrapiv2.ClusterConfig, peers []stnrapiv2.FilterRule) *stnrapiv2.FilterConfig {
	if !slices.ContainsFunc(peers, func(p stnrapiv2.FilterRule) bool { return len(p.Ports) > 0 }) {
		return nil
	}
	rules := append(slices.Clone(peers), stnrapiv2.FilterRule{Action: stnrapiv2.FilterActionDeny.String()})
	return &stnrapiv2.FilterConfig{Name: cluster.Name, Rules: rules}
}

// renderPermissionFilters renders the permission filters of a TURN server: one per rendered cluster
// of the server, in the order of the clusters, named "<server>/<cluster>" and admitting the peer
// addresses of the cluster on any port. A permission no rule allows is denied. Other servers have
// no permission filters.
func (r *defaultFilterRenderer) renderPermissionFilters(server *stnrapiv2.ServerConfig, peers map[string][]stnrapiv2.FilterRule) []stnrapiv2.FilterConfig {
	filters := []stnrapiv2.FilterConfig{}
	if server.Type != stnrapiv2.ServerTypeTURN.String() {
		return filters
	}
	portFiltered := false
	for _, cluster := range server.Clusters {
		rules, ok := peers[cluster]
		if !ok {
			continue
		}
		f := stnrapiv2.FilterConfig{Name: server.Name + "/" + cluster, Rules: []stnrapiv2.FilterRule{}}
		for _, rule := range rules {
			portFiltered = portFiltered || len(rule.Ports) > 0
			rule.Ports = nil
			if !slices.ContainsFunc(f.Rules, func(fr stnrapiv2.FilterRule) bool {
				return fr.IPBlock.CIDR == rule.IPBlock.CIDR
			}) {
				f.Rules = append(f.Rules, rule)
			}
		}
		filters = append(filters, f)
	}

	// only the first UDP cluster of a server makes relay sockets, so its port filter applies to
	// every peer of the server
	if portFiltered && len(server.Clusters) > 1 {
		r.log.Info("Warning: a route of a listener with multiple routes restricts the peer "+
			"ports, the first route's port filter applies to every peer of the listener",
			"listener", server.Name, "routes", server.Clusters)
	}

	return filters
}
