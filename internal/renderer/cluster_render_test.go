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

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderClusterRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "backend found - legacy endpoints controller",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				config.EndpointSliceAvailable = false
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				_, err := r.renderCluster(ro)
				// we have a non-critical error!
				assert.Nil(t, err, "no error")
			},
		},
		{
			name: "backend found",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				config.EndpointSliceAvailable = true
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				config.EndpointSliceAvailable = true
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				_, err := r.renderCluster(ro)
				// we have a non-critical error!
				assert.Nil(t, err, "no error")
			},
		},
		{
			name:   "backend found for V1A2 route - legacy endpoints controller",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			rsV1A2: []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:   []corev1.Service{testutils.TestSvc},
			eps:    []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				config.EndpointSliceAvailable = false
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				_, err := r.renderCluster(ro)
				// we have a non-critical error!
				assert.Nil(t, err, "no error")
			},
		},
		{
			name:   "backend found for V1A2 route",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			rsV1A2: []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:   []corev1.Service{testutils.TestSvc},
			esls:   []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				config.EndpointSliceAvailable = true
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				_, err := r.renderCluster(ro)
				// we have a non-critical error!
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				config.EndpointSliceAvailable = true
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				rc, err := r.renderCluster(rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4:<100-200>", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5:<100-200>", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6:<100-200>", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7:<100-200>", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "4.3.2.1:<100-200>", "endpoint ip-4")
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				config.EndpointSliceAvailable = true
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.False(t, accepted, "route accepted")
			},
		},
		{
			name: "no EDS - cluster ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.testnamespace.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - no backend errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{}
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - wrong backend group ignored",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				group := gwapiv1.Group("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Group = &group
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, InvalidBackendGroup), "invalid backend error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - wrong backend kind ignored",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				kind := gwapiv1.Kind("dummy")
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs[0].Kind = &kind
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, InvalidBackendKind), "invalid backend error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - namespace ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gwapiv1.Namespace("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.dummy.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - multiple backends ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
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
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 3, "endpoints len")
				assert.Contains(t, rc.Endpoints,
					"testservice-ok-1.dummy-ns.svc.cluster.local",
					"backend-ref-1")
				assert.Contains(t, rc.Endpoints,
					"testservice-ok-2.testnamespace.svc.cluster.local",
					"backend-ref-2")
				assert.Contains(t, rc.Endpoints, "dummy.dummy-ns.svc.cluster.local",
					rc.Endpoints[0], "backend-ref-3")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with clusterIP relaying switched off",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with no ClusterIP ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, ClusterIPNotFound), "invalid clusterip error")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with ClusterIP ok",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "4.3.2.1", "cluster-ip")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name:   "eds - cluster for UDPRouteV1A2 and ClusterIP ok",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			rsV1A2: []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:   []corev1.Service{testutils.TestSvc},
			esls:   []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "4.3.2.1", "cluster-ip")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with headless setvice OK",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, ClusterIPNotFound), "invalid clusterip error")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - no backend errs",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - wrong backend group ignored",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, InvalidBackendGroup), "invalid backend error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - wrong backend kind ignored",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, InvalidBackendKind), "invalid backend error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - namespace ok",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				// and the clusterIP
				assert.Contains(t, rc.Endpoints, "1.1.1.1", "cluster-ip")

				// restore EDS
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - multiple backends ok",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(rs[0])
				// no endpoint for dummy svc: handle non-critical error!
				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 8, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "1.2.3.8", "endpoint ip-5")
				// plus the clusterIPs
				assert.Contains(t, rc.Endpoints, "1.1.1.1", "endpoint cluster-ip-1")
				assert.Contains(t, rc.Endpoints, "2.2.2.2", "endpoint cluster-ip-2")
				assert.Contains(t, rc.Endpoints, "3.3.3.3", "endpoint cluster-ip-3")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - multiple backends for UDPRoute V1A2 ok",
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
				c.rsV1A2 = []stnrgwv1.UDPRoute{*udp}

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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(rs[0])
				// no endpoint for dummy svc: handle non-critical error!
				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 8, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "1.2.3.8", "endpoint ip-5")
				// plus the clusterIPs
				assert.Contains(t, rc.Endpoints, "1.1.1.1", "endpoint cluster-ip-1")
				assert.Contains(t, rc.Endpoints, "2.2.2.2", "endpoint cluster-ip-2")
				assert.Contains(t, rc.Endpoints, "3.3.3.3", "endpoint cluster-ip-3")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - multiple backends - missing backends skipped",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(rs[0])
				// no endpoint for dummy svc: handle non-critical error!
				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				// plus the clusterIP
				assert.Contains(t, rc.Endpoints, "1.1.1.1", "endpoint cluster-ip-1")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				rc, err := r.renderCluster(rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 3, "endpoints len")
				// static svc
				assert.Contains(t, rc.Endpoints, "10.11.12.13", "StaticService endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "10.11.12.14", "StaticService endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "10.11.12.15", "StaticService endpoint ip-3")
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				_, err := r.renderCluster(rs[0])
				assert.Error(t, err, "render cluster")

				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, BackendNotFound), "backend not found")
			},
		},
		{
			name:  "Mixed cluster type errs",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			esls:  []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
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
				}, {
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name: "testservice-ok",
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				// switch EDS off: would render a DNS cluster plus a STATIC for the
				// StaticService
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				_, err := r.renderCluster(rs[0])
				assert.Error(t, err, "render cluster")
				assert.True(t, IsNonCritical(err), "critical error")
				assert.True(t, IsNonCriticalError(err, InconsitentClusterType), "inconsistent type")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "Service (w/ EDS) plus StaticService ok - legacy endpoints controller",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = false

				rc, err := r.renderCluster(rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 9, "endpoints len")
				// static svc
				assert.Contains(t, rc.Endpoints, "10.11.12.13", "StaticService 1 endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "10.11.12.14", "StaticService 1 endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "10.11.12.15", "StaticService 1 endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "0.0.0.0/1", "StaticService 2 endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "128.0.0.0/1", "StaticService 2 endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "Service endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "Service endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "Service endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "Service endpoint ip-4")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "Service (w/ EDS) plus StaticService ok",
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				rs := r.allUDPRoutes()
				assert.Len(t, rs, 1, "route len")

				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = false
				config.EndpointSliceAvailable = true

				rc, err := r.renderCluster(rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 9, "endpoints len")
				// static svc
				assert.Contains(t, rc.Endpoints, "10.11.12.13", "StaticService 1 endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "10.11.12.14", "StaticService 1 endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "10.11.12.15", "StaticService 1 endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "0.0.0.0/1", "StaticService 2 endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "128.0.0.0/1", "StaticService 2 endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "Service endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "Service endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "Service endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "Service endpoint ip-4")

				// restore
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
	})
}
