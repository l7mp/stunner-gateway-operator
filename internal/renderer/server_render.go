package renderer

import (
	"fmt"

	"github.com/go-logr/logr"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

var _ serverRenderer = &defaultServerRenderer{}

type defaultServerRenderer struct{ log logr.Logger }

func newServerRenderer(log logr.Logger) serverRenderer {
	return &defaultServerRenderer{log: log}
}

// renderServer renders the TURN server a Gateway listener feeds, named after the listener, naming
// the clusters of the routes of the listener in order and the default auth policy. Its permission
// filters are rendered once the clusters are known.
func (r *defaultServerRenderer) renderServer(c *RenderContext, l *gwapiv1.Listener, rs []store.Route) (*stnrapiv2.ServerConfig, error) {
	if c.gws.Len() != 1 {
		return nil, fmt.Errorf("Internal error: Invalid call to serverRender with %d gws", c.gws.Len())
	}
	gw := c.gws.GetFirst()

	server := stnrapiv2.ServerConfig{
		ServerSpec: stnrapiv2.ServerSpec{
			Name:     stnrListenerName(gw, l),
			Type:     stnrapiv2.ServerTypeTURN.String(),
			Clusters: []string{},
		},
		TURN: &stnrapiv2.TURNServerConfig{Auth: stnrapiv2.DefaultAuthName},
	}
	for _, ro := range rs {
		server.Clusters = append(server.Clusters, clusterName(ro))
	}

	r.log.V(2).Info("Finished rendering server config", "gateway", store.GetObjectKey(gw),
		"listener", l.Name, "result", server.String())

	return &server, nil
}
