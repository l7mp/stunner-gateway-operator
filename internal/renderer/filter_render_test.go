package renderer

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
)

func TestRenderClusterFilter(t *testing.T) {
	r := newFilterRenderer(logr.Discard())
	cluster := &stnrapiv2.ClusterConfig{Name: "testnamespace/udproute-ok/udp"}
	allow := func(cidr string, ports ...stnrapiv2.FilterPort) stnrapiv2.FilterRule {
		return stnrapiv2.FilterRule{Action: "ALLOW", IPBlock: &stnrapiv2.IPBlock{CIDR: cidr}, Ports: ports}
	}

	// no backend restricts its ports: no filter
	assert.Nil(t, r.renderClusterFilter(cluster, []stnrapiv2.FilterRule{allow("1.2.3.4/32")}), "no ports")
	assert.Nil(t, r.renderClusterFilter(cluster, nil), "no peers")

	// a port range: the peers on their ports, then a deny
	f := r.renderClusterFilter(cluster, []stnrapiv2.FilterRule{
		allow("1.2.3.4/32", stnrapiv2.FilterPort{Port: 100, EndPort: 200}),
		allow("10.0.0.0/8"),
	})
	assert.NotNil(t, f, "port filter")
	assert.Equal(t, cluster.Name, f.Name, "named after the cluster")
	assert.Equal(t, []stnrapiv2.FilterRule{
		allow("1.2.3.4/32", stnrapiv2.FilterPort{Port: 100, EndPort: 200}),
		allow("10.0.0.0/8"),
		{Action: "DENY"},
	}, f.Rules, "rules")
}

func TestRenderPermissionFilters(t *testing.T) {
	r := newFilterRenderer(logr.Discard())
	allow := func(cidr string, ports ...stnrapiv2.FilterPort) stnrapiv2.FilterRule {
		return stnrapiv2.FilterRule{Action: "ALLOW", IPBlock: &stnrapiv2.IPBlock{CIDR: cidr}, Ports: ports}
	}
	server := &stnrapiv2.ServerConfig{ServerSpec: stnrapiv2.ServerSpec{Name: "ns/gw/l", Type: "TURN",
		Clusters: []string{"ns/r2/udp", "ns/missing/udp", "ns/r1/udp"}}}
	peers := map[string][]stnrapiv2.FilterRule{
		"ns/r1/udp": {allow("1.2.3.4/32")},
		// the same block on two port ranges admits the block twice, on any port
		"ns/r2/udp": {
			allow("10.0.0.0/8", stnrapiv2.FilterPort{Port: 100}),
			allow("10.0.0.0/8", stnrapiv2.FilterPort{Port: 200, EndPort: 300}),
		},
	}

	// a filter per rendered cluster, in the order of the clusters, and no default deny
	fs := r.renderPermissionFilters(server, peers)
	assert.Equal(t, []stnrapiv2.FilterConfig{
		{Name: "ns/gw/l/ns/r2/udp", Rules: []stnrapiv2.FilterRule{allow("10.0.0.0/8"), allow("10.0.0.0/8")}},
		{Name: "ns/gw/l/ns/r1/udp", Rules: []stnrapiv2.FilterRule{allow("1.2.3.4/32")}},
	}, fs, "permission filters")

	// an L4 server has no permission filters
	l4 := &stnrapiv2.ServerConfig{ServerSpec: stnrapiv2.ServerSpec{Name: "ns/gw/l4", Type: "L4",
		Clusters: []string{"ns/r1/udp"}}}
	assert.Empty(t, r.renderPermissionFilters(l4, peers), "L4 server")

	// a server without clusters gets none, denying every peer
	server.Clusters = []string{}
	assert.Empty(t, r.renderPermissionFilters(server, peers), "no clusters")
}
