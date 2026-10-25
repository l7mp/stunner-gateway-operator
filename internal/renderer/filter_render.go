package renderer

import (
	"github.com/go-logr/logr"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
)

var _ filterRenderer = &defaultFilterRenderer{}

type defaultFilterRenderer struct{ log logr.Logger }

func newFilterRenderer(log logr.Logger) filterRenderer {
	return &defaultFilterRenderer{log: log}
}

// renderServerFilters renders the filters of a server relaying to the peers others choose, a TURN
// or an RTP server: one per rendered cluster of the server, in the order of the clusters, named
// "<server>/<cluster>" and admitting the peer addresses of the cluster on the port range of their
// backend, if any (every address for the cluster of a TURNRoute). A peer no rule allows is
// refused. Other servers have no filters.
func (r *defaultFilterRenderer) renderServerFilters(server *stnrapiv2.ServerConfig, peers map[string][]stnrapiv2.FilterRule) []stnrapiv2.FilterConfig {
	filters := []stnrapiv2.FilterConfig{}
	if server.Type != stnrapiv2.ServerTypeTURN.String() && server.Type != stnrapiv2.ServerTypeRTP.String() {
		return filters
	}
	tunnel := false
	for _, cluster := range server.Clusters {
		rules, ok := peers[cluster]
		if !ok {
			continue
		}
		f := stnrapiv2.FilterConfig{Name: server.Name + "/" + cluster, Rules: []stnrapiv2.FilterRule{}}
		for _, rule := range rules {
			tunnel = tunnel || rule.IPBlock == nil
			f.Rules = append(f.Rules, rule)
		}
		filters = append(filters, f)
	}
	if tunnel && len(server.Clusters) > 1 {
		r.log.Info("Warning: a TURNRoute shares a listener with other routes, the first route "+
			"makes the relay sockets of every client of the listener",
			"listener", server.Name, "routes", server.Clusters)
	}

	return filters
}
