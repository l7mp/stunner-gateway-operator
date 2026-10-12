package renderer

import (
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderClusterRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "backend found",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				_, _, err := r.renderCluster(nil, ro)
				assert.Nil(t, err, "no error")
			},
		},
		{
			name:    "backend found for V1A2 route",
			cls:     []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:     []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:     []gwapiv1.Gateway{testutils.TestGw},
			rsGwAPI: []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:    []corev1.Service{testutils.TestSvc},
			esls:    []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				_, _, err := r.renderCluster(nil, ro)
				assert.Nil(t, err, "no error")
			},
		},
		{
			name: "default with port range considered",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				port := gwapiv1.PortNumber(100)
				endPort := gwapiv1.PortNumber(200)
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name:      gwapiv1.ObjectName(testutils.TestSvc.GetName()),
						Namespace: &testutils.TestNsName,
						Port:      &port,
						EndPort:   &endPort,
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Equal(t, []string{"$STUNNER_ADDR"}, rc.Addrs, "relay address")
				assert.Equal(t, []string{"1.2.3.4:100", "1.2.3.5:100", "1.2.3.6:100", "1.2.3.7:100"},
					rc.Endpoints, "endpoints at the backend port, no ClusterIP")

				// the port range goes to the peers, never to the endpoints
				assert.Len(t, peers, 5, "peers len")
				for _, p := range peers {
					assert.Equal(t, "ALLOW", p.Action, "peer action")
					assert.Equal(t, []stnrapiv2.FilterPort{{Port: 100, EndPort: 200}}, p.Ports, "peer ports")
				}
				assert.Equal(t, "1.2.3.4/32", peers[0].IPBlock.CIDR, "peer block")
			},
		},
		{
			name: "a bare backend port restricts nothing",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				port := gwapiv1.PortNumber(1)
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name:      gwapiv1.ObjectName(testutils.TestSvc.GetName()),
						Namespace: &testutils.TestNsName,
						Port:      &port,
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
				svc := testutils.TestSvc.DeepCopy()
				svc.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*svc}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				_, peers, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")
				assert.NotEmpty(t, peers, "peers")
				for _, p := range peers {
					assert.Empty(t, p.Ports, "a port without endPort is no port range")
				}
			},
		},
		{
			name: "the backend port is the port of the endpoints",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				port, endPort := gwapiv1.PortNumber(5000), gwapiv1.PortNumber(6000)
				udp.Spec.Rules[0].BackendRefs[0].Port = &port
				udp.Spec.Rules[0].BackendRefs[0].EndPort = &endPort
				c.rs = []stnrgwv1.UDPRoute{*udp}
				svc := testutils.TestSvc.DeepCopy()
				svc.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*svc}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				// the addresses at the backend port, the port range only in the filter rules
				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, []string{"1.2.3.4:5000", "1.2.3.5:5000", "1.2.3.6:5000", "1.2.3.7:5000"},
					rc.Endpoints, "endpoints, no ClusterIP")
				assert.Equal(t, []string{"$STUNNER_ADDR"}, rc.Addrs, "relay address")
				assert.Len(t, peers, 5, "peers len")
				for _, p := range peers {
					assert.Equal(t, []stnrapiv2.FilterPort{{Port: 5000, EndPort: 6000}}, p.Ports, "peer ports")
				}
			},
		},
		{
			name:  "StaticService endpoints at the backend port",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			ssvcs: []stnrgwv1.StaticService{testutils.TestStaticSvc},
			prep: func(c *renderTestConfig) {
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				port := gwapiv1.PortNumber(7000)
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
						Port:  &port,
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
				ssvc := testutils.TestStaticSvc.DeepCopy()
				ssvc.Spec.Prefixes = []string{"10.11.12.13", "10.0.0.0/8", "1.2.3.4:<100-200>"}
				c.ssvcs = []stnrgwv1.StaticService{*ssvc}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				// single addresses at the backend port; prefixes and port ranges only in the
				// filter rules, a bare backend port is no port range
				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")
				assert.Equal(t, []string{"1.2.3.4:7000", "10.11.12.13:7000"}, rc.Endpoints, "endpoints")
				assert.Len(t, peers, 3, "peers len")
				assert.Empty(t, peers[0].Ports, "10.11.12.13")
				assert.Equal(t, "10.0.0.0/8", peers[1].IPBlock.CIDR, "the prefix")
				assert.Equal(t, []stnrapiv2.FilterPort{{Port: 100, EndPort: 200}}, peers[2].Ports, "the prefix range")
			},
		},
		{
			name: "linking to a foreign gateway errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.GatewayClassName = gwapiv1.ObjectName("dummy")
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.False(t, exists, "route exists")
				assert.False(t, accepted, "route accepted")
			},
		},
		{
			name: "cluster with clusterIP relaying switched off",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = false

				rc, peers, err := r.renderCluster(nil, ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.NotContains(t, peerBlocks(peers), "4.3.2.1/32", "cluster-ip: not a peer")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "cluster with no ClusterIP ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, _, err := r.renderCluster(nil, ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, ClusterIPNotFound), "invalid clusterip error")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "cluster with ClusterIP ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, peers, err := r.renderCluster(nil, ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, peerBlocks(peers), "4.3.2.1/32", "cluster-ip: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name:    "cluster for UDPRouteV1A2 and ClusterIP ok",
			cls:     []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:     []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:     []gwapiv1.Gateway{testutils.TestGw},
			rsGwAPI: []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:    []corev1.Service{testutils.TestSvc},
			esls:    []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, peers, err := r.renderCluster(nil, ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, peerBlocks(peers), "4.3.2.1/32", "cluster-ip: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "cluster with headless service ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "None"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, _, err := r.renderCluster(nil, ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, ClusterIPNotFound), "invalid clusterip error")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no backend errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{}
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, _, err := r.renderCluster(nil, ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-wrong/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "wrong backend group ignored",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				group := gwapiv1.Group("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Group = &group
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, _, err := r.renderCluster(nil, ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, InvalidBackendGroup), "invalid backend error")

				assert.Equal(t, "testnamespace/udproute-wrong/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "wrong backend kind ignored",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				kind := gwapiv1.Kind("dummy")
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs[0].Kind = &kind
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, _, err := r.renderCluster(nil, ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, InvalidBackendKind), "invalid backend error")

				assert.Equal(t, "testnamespace/udproute-wrong/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "namespace ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gwapiv1.Namespace("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				c.rs = []stnrgwv1.UDPRoute{*udp}

				s1 := testutils.TestSvc.DeepCopy()
				s1.SetNamespace("dummy")
				// add a clusterIP to silence renderCluster
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}

				e := testutils.TestEndpointSlice.DeepCopy()
				e.SetNamespace("dummy")
				c.esls = []discoveryv1.EndpointSlice{*e}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				config.EnableRelayToClusterIP = true

				rc, peers, err := r.renderCluster(nil, ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				// and the clusterIP
				assert.Contains(t, peerBlocks(peers), "1.1.1.1/32", "cluster-ip: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "multiple backends ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gwapiv1.Namespace("dummy-ns")
				udp.Spec.Rules[0].BackendRefs = make([]stnrgwv1.BackendRef, 3)
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[0].Name = "dummy"
				udp.Spec.Rules[0].BackendRefs[1].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[1].Name = "testservice-ok-1"
				udp.Spec.Rules[0].BackendRefs[2].Name = "testservice-ok-2"
				c.rs = []stnrgwv1.UDPRoute{*udp}

				s1 := testutils.TestSvc.DeepCopy()
				s1.SetNamespace("dummy-ns")
				s1.SetName("dummy")
				// add a clusterIP to silence renderCluster
				s1.Spec.ClusterIP = "1.1.1.1"

				s2 := testutils.TestSvc.DeepCopy()
				s2.SetNamespace("dummy-ns")
				s2.SetName("testservice-ok-1")
				// add a clusterIP to silence renderCluster
				s2.Spec.ClusterIP = "2.2.2.2"

				s3 := testutils.TestSvc.DeepCopy()
				s3.SetName("testservice-ok-2")
				// add a clusterIP to silence renderCluster
				s3.Spec.ClusterIP = "3.3.3.3"

				c.svcs = []corev1.Service{*s1, *s2, *s3}

				e1 := testutils.TestEndpointSlice.DeepCopy()
				e1.SetNamespace("dummy-ns")
				e1.SetName("testservice-ok-endpointslice-1")
				e1.SetLabels(map[string]string{"kubernetes.io/service-name": "testservice-ok-1"})

				e2 := testutils.TestEndpointSlice.DeepCopy()
				e2.SetName("testservice-ok-endpointslice-2")
				e2.SetLabels(map[string]string{"kubernetes.io/service-name": "testservice-ok-2"})
				e2.Endpoints = []discoveryv1.Endpoint{{
					Addresses: []string{"1.2.3.8"},
					Conditions: discoveryv1.EndpointConditions{
						Ready:   &testutils.TestTrue,
						Serving: &testutils.TestTrue,
					},
				}}
				c.esls = []discoveryv1.EndpointSlice{*e1, *e2}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				config.EnableRelayToClusterIP = true

				rc, peers, err := r.renderCluster(nil, rs[0])
				// no endpoint for dummy svc: handle non-critical error!
				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "1.2.3.8", "endpoint ip-5")
				// plus the clusterIPs
				assert.Contains(t, peerBlocks(peers), "1.1.1.1/32", "cluster-ip-1: a peer, no endpoint")
				assert.Contains(t, peerBlocks(peers), "2.2.2.2/32", "cluster-ip-2: a peer, no endpoint")
				assert.Contains(t, peerBlocks(peers), "3.3.3.3/32", "cluster-ip-3: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "multiple backends for UDPRoute V1A2 ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gwapiv1.Namespace("dummy-ns")
				udp.Spec.Rules[0].BackendRefs = make([]stnrgwv1.BackendRef, 3)
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[0].Name = "dummy"
				udp.Spec.Rules[0].BackendRefs[1].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[1].Name = "testservice-ok-1"
				udp.Spec.Rules[0].BackendRefs[2].Name = "testservice-ok-2"
				c.rsGwAPI = []stnrgwv1.UDPRoute{*udp}

				s1 := testutils.TestSvc.DeepCopy()
				s1.SetNamespace("dummy-ns")
				s1.SetName("dummy")
				// add a clusterIP to silence renderCluster
				s1.Spec.ClusterIP = "1.1.1.1"

				s2 := testutils.TestSvc.DeepCopy()
				s2.SetNamespace("dummy-ns")
				s2.SetName("testservice-ok-1")
				// add a clusterIP to silence renderCluster
				s2.Spec.ClusterIP = "2.2.2.2"

				s3 := testutils.TestSvc.DeepCopy()
				s3.SetName("testservice-ok-2")
				// add a clusterIP to silence renderCluster
				s3.Spec.ClusterIP = "3.3.3.3"

				c.svcs = []corev1.Service{*s1, *s2, *s3}

				e1 := testutils.TestEndpointSlice.DeepCopy()
				e1.SetNamespace("dummy-ns")
				e1.SetName("testservice-ok-endpointslice-1")
				e1.SetLabels(map[string]string{"kubernetes.io/service-name": "testservice-ok-1"})

				e2 := testutils.TestEndpointSlice.DeepCopy()
				e2.SetName("testservice-ok-endpointslice-2")
				e2.SetLabels(map[string]string{"kubernetes.io/service-name": "testservice-ok-2"})
				e2.Endpoints = []discoveryv1.Endpoint{{
					Addresses: []string{"1.2.3.8"},
					Conditions: discoveryv1.EndpointConditions{
						Ready:   &testutils.TestTrue,
						Serving: &testutils.TestTrue,
					},
				}}
				c.esls = []discoveryv1.EndpointSlice{*e1, *e2}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				config.EnableRelayToClusterIP = true

				rc, peers, err := r.renderCluster(nil, rs[0])
				// no endpoint for dummy svc: handle non-critical error!
				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "1.2.3.8", "endpoint ip-5")
				// plus the clusterIPs
				assert.Contains(t, peerBlocks(peers), "1.1.1.1/32", "cluster-ip-1: a peer, no endpoint")
				assert.Contains(t, peerBlocks(peers), "2.2.2.2/32", "cluster-ip-2: a peer, no endpoint")
				assert.Contains(t, peerBlocks(peers), "3.3.3.3/32", "cluster-ip-3: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "multiple backends - missing backends skipped",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gwapiv1.Namespace("dummy-ns")
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{
					{
						BackendObjectReference: stnrgwv1.BackendObjectReference{
							// missing svc
							Name: "dummy",
						},
					},
					{
						BackendObjectReference: stnrgwv1.BackendObjectReference{
							// no clusterip, has endpoints
							Namespace: &ns,
							Name:      "testservice-ok-1",
						},
					},
					{
						BackendObjectReference: stnrgwv1.BackendObjectReference{
							// has clusterip, no endpoints
							Namespace: &ns,
							Name:      "testservice-ok-2",
						},
					},
				}
				c.rs = []stnrgwv1.UDPRoute{*udp}

				s1 := testutils.TestSvc.DeepCopy()
				s1.SetName("testservice-ok-1")
				s1.SetNamespace("dummy-ns")
				// no clusterIP but has endpoints

				s2 := testutils.TestSvc.DeepCopy()
				s2.SetName("testservice-ok-2")
				s2.SetNamespace("dummy-ns")
				// add a clusterIP but no endpoints
				s2.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1, *s2}

				e := testutils.TestEndpointSlice.DeepCopy()
				e.SetNamespace("dummy-ns")
				e.SetName("testservice-ok-endpointslice")
				e.SetLabels(map[string]string{"kubernetes.io/service-name": "testservice-ok-1"})
				c.esls = []discoveryv1.EndpointSlice{*e}

			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				config.EnableRelayToClusterIP = true

				rc, peers, err := r.renderCluster(nil, rs[0])
				// no endpoint for testservice-ok-2: non-critical EndpointNotFound error
				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				// plus the clusterIP
				assert.Contains(t, peerBlocks(peers), "1.1.1.1/32", "cluster-ip-1: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		// StaticService
		{
			name:  "StaticService ok",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			ssvcs: []stnrgwv1.StaticService{testutils.TestStaticSvc},
			prep: func(c *renderTestConfig) {
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				rc, _, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 3, "endpoints len")
				// static svc
				assert.Contains(t, rc.Endpoints, "10.11.12.13", "StaticService endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "10.11.12.14", "StaticService endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "10.11.12.15", "StaticService endpoint ip-3")
			},
		},
		{
			name:  "StaticService with an invalid address skips the address",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			ssvcs: []stnrgwv1.StaticService{testutils.TestStaticSvc},
			prep: func(c *renderTestConfig) {
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
				ssvc := testutils.TestStaticSvc.DeepCopy()
				ssvc.Spec.Prefixes = []string{"10.11.12.13", "example.com", "10.0.0.0/8"}
				c.ssvcs = []stnrgwv1.StaticService{*ssvc}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.True(t, IsNonCriticalError(err, InvalidEndpoint), "invalid endpoint error")
				assert.NotNil(t, rc, "cluster rendered")
				assert.Equal(t, []string{"10.11.12.13"}, rc.Endpoints, "endpoints")
				assert.Len(t, peers, 2, "peers len")
				assert.Equal(t, "10.11.12.13/32", peers[0].IPBlock.CIDR, "peer block-1")
				assert.Equal(t, "10.0.0.0/8", peers[1].IPBlock.CIDR, "peer block-2")
			},
		},
		{
			name:  "StaticService prefixes with port ranges",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			ssvcs: []stnrgwv1.StaticService{testutils.TestStaticSvc},
			prep: func(c *renderTestConfig) {
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				port, endPort := gwapiv1.PortNumber(1000), gwapiv1.PortNumber(1100)
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group:   &group,
						Kind:    &kind,
						Name:    "teststaticservice-ok",
						Port:    &port,
						EndPort: &endPort,
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
				ssvc := testutils.TestStaticSvc.DeepCopy()
				ssvc.Spec.Prefixes = []string{"10.11.12.13", "10.0.0.0/8:<5000-6000>",
					"1.2.3.4:<1-65535>", "5.6.7.8:<9-3>"}
				c.ssvcs = []stnrgwv1.StaticService{*ssvc}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.True(t, IsNonCriticalError(err, InvalidPortRange), "invalid port range error")
				assert.NotNil(t, rc, "cluster rendered")
				assert.Equal(t, []string{"1.2.3.4:1000", "10.11.12.13:1000"}, rc.Endpoints,
					"endpoints at the backend port")

				// the backend port, the prefix's own range, and the full range of the prefix
				assert.Len(t, peers, 3, "peers len")
				assert.Equal(t, "10.11.12.13/32", peers[0].IPBlock.CIDR, "peer block-1")
				assert.Equal(t, []stnrapiv2.FilterPort{{Port: 1000, EndPort: 1100}}, peers[0].Ports, "peer ports-1")
				assert.Equal(t, "10.0.0.0/8", peers[1].IPBlock.CIDR, "peer block-2")
				assert.Equal(t, []stnrapiv2.FilterPort{{Port: 5000, EndPort: 6000}}, peers[1].Ports, "peer ports-2")
				assert.Equal(t, "1.2.3.4/32", peers[2].IPBlock.CIDR, "peer block-3")
				assert.Empty(t, peers[2].Ports, "peer ports-3")
			},
		},
		{
			name: "No StaticService backend errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			prep: func(c *renderTestConfig) {
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-dummy",
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				_, _, err := r.renderCluster(nil, rs[0])
				assert.Error(t, err, "render cluster")

				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, BackendNotFound), "backend not found")
			},
		},
		{
			name: "Service plus StaticService ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
					},
				}, {
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice2",
					},
				}, {
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name: "testservice-ok",
					},
				}}

				c.rs = []stnrgwv1.UDPRoute{*udp}

				ssvc2 := testutils.TestStaticSvc.DeepCopy()
				ssvc2.SetName("teststaticservice2")
				ssvc2.Spec.Prefixes = []string{"0.0.0.0/1", "128.0.0.0/1"}
				c.ssvcs = []stnrgwv1.StaticService{testutils.TestStaticSvc, *ssvc2}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")

				config.EnableRelayToClusterIP = false

				rc, peers, err := r.renderCluster(nil, rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				// a prefix is a peer block but no endpoint
				assert.Len(t, rc.Endpoints, 7, "endpoints len")
				assert.Len(t, peers, 9, "peers len")
				// static svc
				assert.Contains(t, rc.Endpoints, "10.11.12.13", "StaticService 1 endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "10.11.12.14", "StaticService 1 endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "10.11.12.15", "StaticService 1 endpoint ip-3")
				blocks := []string{}
				for _, p := range peers {
					blocks = append(blocks, p.IPBlock.CIDR)
					assert.Empty(t, p.Ports, "no port restriction")
				}
				assert.Contains(t, blocks, "0.0.0.0/1", "StaticService 2 block-1")
				assert.Contains(t, blocks, "128.0.0.0/1", "StaticService 2 block-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "Service endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "Service endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "Service endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "Service endpoint ip-4")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
	})
}
