package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderTURNRouteRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name:   "TURNRoute cluster ok",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			turnrs: []stnrgwv1.TURNRoute{testutils.TestTURNRoute},
			tscrts: []corev1.Secret{testutils.TestTunnelSecret},
			prep:   func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")
				assert.Equal(t, "testnamespace/turnroute-ok/turn", rc.Name, "cluster name")
				assert.Equal(t, "UDP", rc.Protocol, "protocol")
				assert.Empty(t, rc.Endpoints, "no backends, no endpoints")
				assert.NotNil(t, rc.Tunnel, "tunnel")
				assert.Equal(t, "turns:turn.example.com:443?transport=tcp", rc.Tunnel.URL, "tunnel URL")
				assert.Equal(t, rc.Name, rc.Tunnel.Auth, "tunnel auth")
				assert.NotNil(t, rc.Tunnel.TLS, "tunnel TLS")
				assert.True(t, rc.Tunnel.TLS.Insecure, "insecure")
				assert.Equal(t, "sni.example.com", rc.Tunnel.TLS.SNI, "SNI")

				// the tunnel admits every peer
				assert.Equal(t, []stnrapiv2.FilterRule{{Action: "ALLOW"}}, peers, "peers")

				auth, err := renderTunnelAuth(rs[0])
				assert.NoError(t, err, "render tunnel auth")
				assert.Equal(t, rc.Name, auth.Name, "auth name")
				assert.Equal(t, "static", auth.Type, "auth type")
				assert.Equal(t, "upstream.example.com", auth.Realm, "realm")
				assert.Equal(t, map[string]string{"username": "tunnel-user", "password": "tunnel-pass"},
					auth.Credentials, "credentials")
			},
		},
		{
			name:   "TURNRoute with backends",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			turnrs: []stnrgwv1.TURNRoute{testutils.TestTURNRoute},
			tscrts: []corev1.Secret{testutils.TestTunnelSecret},
			svcs:   []corev1.Service{testutils.TestSvc},
			esls:   []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				ro := testutils.TestTURNRoute.DeepCopy()
				port := gwapiv1.PortNumber(5000)
				ro.Spec.Rules = []stnrgwv1.RouteRule{{BackendRefs: []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name: gwapiv1.ObjectName(testutils.TestSvc.GetName()),
						Port: &port,
					},
				}}}}
				c.turnrs = []stnrgwv1.TURNRoute{*ro}
			},
			tester: func(t *testing.T, r *renderer) {
				config.EnableRelayToClusterIP = false
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				// the backends at their port, dialed through the tunnel by an L4 server
				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")
				assert.Equal(t, "testnamespace/turnroute-ok/turn", rc.Name, "cluster name")
				assert.Equal(t, []string{"1.2.3.4:5000", "1.2.3.5:5000"},
					rc.Endpoints, "endpoints")
				assert.NotNil(t, rc.Tunnel, "tunnel")
				assert.Equal(t, []stnrapiv2.FilterRule{{Action: "ALLOW"}}, peers, "peers")
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name:   "TURNRoute errors",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			turnrs: []stnrgwv1.TURNRoute{testutils.TestTURNRoute},
			prep:   func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				// no Secret
				ro := testutils.TestTURNRoute.DeepCopy()
				rc, _, err := r.renderCluster(nil, ro)
				assert.Nil(t, rc, "no cluster")
				assert.True(t, IsNonCriticalError(err, TunnelAuthNotFound), "missing Secret")

				// not a TURN URI
				ro.Spec.Tunnel.URI = "stun:turn.example.com:3478"
				rc, _, err = r.renderCluster(nil, ro)
				assert.Nil(t, rc, "no cluster")
				assert.True(t, IsNonCriticalError(err, InvalidTunnelURI), "invalid URI")

				// no Secret needed without an authRef
				ro.Spec.Tunnel.URI = "turn:turn.example.com:3478"
				ro.Spec.Tunnel.AuthRef = nil
				rc, _, err = r.renderCluster(nil, ro)
				assert.NoError(t, err, "no authRef")
				assert.Equal(t, "", rc.Tunnel.Auth, "no tunnel auth")
				auth, err := renderTunnelAuth(ro)
				assert.NoError(t, err, "no authRef")
				assert.Nil(t, auth, "no auth")

				// the Secret of an ephemeral auth, an unknown type
				secret := testutils.TestTunnelSecret.DeepCopy()
				secret.Data = map[string][]byte{"type": []byte("ephemeral"), "secret": []byte("s"),
					"lifetime": []byte("2h")}
				store.TunnelAuthSecrets.Upsert(secret)
				ro.Spec.Tunnel.AuthRef = testutils.TestTURNRoute.Spec.Tunnel.AuthRef
				auth, err = renderTunnelAuth(ro)
				assert.NoError(t, err, "ephemeral")
				assert.Equal(t, "ephemeral", auth.Type, "auth type")
				assert.Equal(t, "2h", auth.Lifetime, "lifetime")
				assert.Equal(t, map[string]string{"secret": "s"}, auth.Credentials, "credentials")

				secret.Data["type"] = []byte("dummy")
				store.TunnelAuthSecrets.Upsert(secret)
				_, err = renderTunnelAuth(ro)
				assert.True(t, IsNonCriticalError(err, InvalidTunnelAuth), "unknown type")
			},
		},
		{
			name:   "TURNRoute and UDPRoute on a TURN listener - E2E test",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			rs:     []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			turnrs: []stnrgwv1.TURNRoute{testutils.TestTURNRoute},
			tscrts: []corev1.Secret{testutils.TestTunnelSecret},
			svcs:   []corev1.Service{testutils.TestSvc},
			esls:   []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:    []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep:   func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))
				assert.NoError(t, r.renderForGateways(c), "render success")

				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]
				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")

				// the default policy and the tunnel's
				assert.Len(t, conf.Auths, 2, "auth num")
				auth, err := conf.GetAuthConfig("testnamespace/turnroute-ok/turn")
				assert.NoError(t, err, "tunnel auth")
				assert.Equal(t, "tunnel-user", auth.Credentials["username"], "tunnel credentials")

				// both clusters on the listener: a valid config, only a warning
				assert.Len(t, conf.Clusters, 2, "cluster num")
				tc, err := conf.GetClusterConfig("testnamespace/turnroute-ok/turn")
				assert.NoError(t, err, "tunnel cluster")
				assert.Equal(t, tc.Name, tc.Tunnel.Auth, "tunnel auth")

				lc, err := conf.GetListenerConfig("testnamespace/gateway-1/gateway-1-listener-udp")
				assert.NoError(t, err, "listener")
				sc := serverOf(conf, lc)
				assert.ElementsMatch(t, []string{"testnamespace/udproute-ok/udp",
					"testnamespace/turnroute-ok/turn"}, sc.Clusters, "clusters")

				// the tunnel's server filter admits every peer
				f, err := conf.GetFilterConfig(sc.Name + "/testnamespace/turnroute-ok/turn")
				assert.NoError(t, err, "tunnel server filter")
				assert.Len(t, f.Rules, 1, "tunnel server filter rules")
				assert.Equal(t, "ALLOW", f.Rules[0].Action, "allow")
				assert.Nil(t, f.Rules[0].IPBlock, "every address")

				// the route resolves
				turnRoutes := 0
				for _, ro := range r.allRoutes() {
					if _, ok := ro.(*stnrgwv1.TURNRoute); !ok {
						continue
					}
					turnRoutes++
					d := meta.FindStatusCondition(ro.GetRouteStatus().Parents[0].Conditions,
						string(gwapiv1.RouteConditionResolvedRefs))
					assert.NotNil(t, d, "resolved-refs found")
					assert.Equal(t, metav1.ConditionTrue, d.Status, "resolved")
				}
				assert.Equal(t, 1, turnRoutes, "TURNRoute num")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
	})
}
