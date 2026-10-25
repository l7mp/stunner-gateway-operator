package renderer

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
)

func TestRenderServerFilters(t *testing.T) {
	r := newFilterRenderer(logr.Discard())
	allow := func(cidr string, ports ...stnrapiv2.FilterPort) stnrapiv2.FilterRule {
		return stnrapiv2.FilterRule{Action: "ALLOW", IPBlock: &stnrapiv2.IPBlock{CIDR: cidr}, Ports: ports}
	}
	server := &stnrapiv2.ServerConfig{ServerSpec: stnrapiv2.ServerSpec{Name: "ns/gw/l", Type: "TURN",
		Clusters: []string{"ns/r2/udp", "ns/missing/udp", "ns/r1/udp"}}}
	peers := map[string][]stnrapiv2.FilterRule{
		"ns/r1/udp": {allow("1.2.3.4/32")},
		"ns/r2/udp": {
			allow("10.0.0.0/8", stnrapiv2.FilterPort{Port: 100}),
			allow("10.0.0.0/8", stnrapiv2.FilterPort{Port: 200, EndPort: 300}),
		},
	}

	// a filter per rendered cluster, in the order of the clusters, the port ranges kept, and no
	// default deny
	want := []stnrapiv2.FilterConfig{
		{Name: "ns/gw/l/ns/r2/udp", Rules: []stnrapiv2.FilterRule{
			allow("10.0.0.0/8", stnrapiv2.FilterPort{Port: 100}),
			allow("10.0.0.0/8", stnrapiv2.FilterPort{Port: 200, EndPort: 300}),
		}},
		{Name: "ns/gw/l/ns/r1/udp", Rules: []stnrapiv2.FilterRule{allow("1.2.3.4/32")}},
	}
	assert.Equal(t, want, r.renderServerFilters(server, peers), "TURN server")

	// an RTP server filters its media peers the same way
	rtp := *server
	rtp.Type = "RTP"
	assert.Equal(t, want, r.renderServerFilters(&rtp, peers), "RTP server")

	// a routing server has no filters
	l4 := &stnrapiv2.ServerConfig{ServerSpec: stnrapiv2.ServerSpec{Name: "ns/gw/l4", Type: "L4",
		Clusters: []string{"ns/r1/udp"}}}
	assert.Empty(t, r.renderServerFilters(l4, peers), "L4 server")

	// a server without clusters gets none, refusing every peer
	server.Clusters = []string{}
	assert.Empty(t, r.renderServerFilters(server, peers), "no clusters")
}
