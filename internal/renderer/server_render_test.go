package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderServerRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "server of a listener ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				// the UDP listener has the route, the TCP listener none
				l := gw.Spec.Listeners[0]
				rs := r.getRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				sc, err := r.renderServer(c, &l, rs)
				assert.NoError(t, err, "renderServer")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", sc.Name, "server name")
				assert.Equal(t, "TURN", sc.Type, "server type")
				assert.Equal(t, "default", sc.TURN.Auth, "auth policy")
				assert.Equal(t, []string{"testnamespace/udproute-ok/udp"}, sc.Clusters, "clusters")
				assert.Empty(t, sc.Filters, "the filters are rendered later")

				l = gw.Spec.Listeners[1]
				rs = r.getRoutes4Listener(gw, &l)
				assert.Empty(t, rs, "no route")
				sc, err = r.renderServer(c, &l, rs)
				assert.NoError(t, err, "renderServer")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", sc.Name, "server name")
				assert.Empty(t, sc.Clusters, "no clusters")

				// a plain protocol feeds an L4 server, naming the L4 clusters of the routes
				l = gw.Spec.Listeners[0]
				l.Protocol = gwapiv1.ProtocolType("UDP")
				sc, err = r.renderServer(c, &l, r.getRoutes4Listener(gw, &l))
				assert.NoError(t, err, "renderServer")
				assert.Equal(t, "L4", sc.Type, "server type")
				assert.Nil(t, sc.TURN, "no TURN block")
				assert.Equal(t, "default", sc.L4.Auth, "auth policy")
				assert.Equal(t, []string{"testnamespace/udproute-ok/udp"}, sc.Clusters, "clusters")

				// an SRT listener renders no server
				l.Protocol = gwapiv1.ProtocolType("SRT")
				_, err = r.renderServer(c, &l, r.getRoutes4Listener(gw, &l))
				assert.True(t, IsNonCriticalError(err, UnsupportedSRT), "SRT is not supported")
			},
		},
	})
}
