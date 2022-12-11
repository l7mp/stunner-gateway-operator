package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderClusterRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "backend found",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
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
			name: "linking to a foreign gateway errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.GatewayClassName = gatewayv1alpha2.ObjectName("dummy")
				c.gws = []gatewayv1alpha2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.False(t, accepted, "route accepted")
			},
		},
		{
			name: "no EDS - cluster ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.testnamespace.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - no backend errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs = []gatewayv1alpha2.BackendRef{}
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - wrong backend group ignored",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				group := gatewayv1alpha2.Group("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Group = &group
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				e, ok := err.(NonCriticalRenderError)
				assert.True(t, ok, "non-critical error")
				assert.Equal(t, InvalidBackendGroup, e.ErrorReason, "invalid route error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - wrong backend kind ignored",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				kind := gatewayv1alpha2.Kind("dummy")
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs[0].Kind = &kind
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				rc, err := r.renderCluster(ro)
				assert.NotNil(t, err, "error")
				e, ok := err.(NonCriticalRenderError)
				assert.True(t, ok, "non-critical error")
				assert.Equal(t, InvalidBackendKind, e.ErrorReason, "invalid kind error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - namespace ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gatewayv1alpha2.Namespace("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.dummy.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no EDS - multiple backends ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gatewayv1alpha2.Namespace("dummy-ns")
				udp.Spec.Rules[0].BackendRefs = make([]gatewayv1alpha2.BackendRef, 3)
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[0].Name = "dummy"
				udp.Spec.Rules[0].BackendRefs[1].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[1].Name = "testservice-ok-1"
				udp.Spec.Rules[0].BackendRefs[2].Name = "testservice-ok-2"
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

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
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with clusterIP relaying switched off",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = false

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
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with no ClusterIP ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				rc, err := r.renderCluster(ro)
				assert.NotNil(t, err, "error")
				e, ok := err.(NonCriticalRenderError)
				assert.True(t, ok, "non-critical error")
				assert.Equal(t, ClusterIPNotFound, e.ErrorReason, "cluster IP not found error")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with ClusterIP ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

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
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - cluster with headless setvice OK",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "None"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				e, ok := err.(NonCriticalRenderError)
				assert.True(t, ok, "non-critical error")
				assert.Equal(t, ClusterIPNotFound, e.ErrorReason, "cluster IP not found error")

				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - no backend errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs = []gatewayv1alpha2.BackendRef{}
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - wrong backend group ignored",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.SetName("udproute-wrong")
				group := gatewayv1alpha2.Group("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Group = &group
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				e, ok := err.(NonCriticalRenderError)
				assert.True(t, ok, "non-critical error")
				assert.Equal(t, InvalidBackendGroup, e.ErrorReason, "invalid backend group error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - wrong backend kind ignored",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				kind := gatewayv1alpha2.Kind("dummy")
				udp.SetName("udproute-wrong")
				udp.Spec.Rules[0].BackendRefs[0].Kind = &kind
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				rc, err := r.renderCluster(ro)

				assert.NotNil(t, err, "error")
				e, ok := err.(NonCriticalRenderError)
				assert.True(t, ok, "non-critical error")
				assert.Equal(t, InvalidBackendKind, e.ErrorReason, "invalid backend kind error")

				assert.Equal(t, "testnamespace/udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")

				// restore
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - namespace ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gatewayv1alpha2.Namespace("dummy")
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}

				s1 := testutils.TestSvc.DeepCopy()
				s1.SetNamespace("dummy")
				// add a clusterIP to silence renderCluster
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}

				e := testutils.TestEndpoint.DeepCopy()
				e.SetNamespace("dummy")
				c.eps = []corev1.Endpoints{*e}
			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

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
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "eds - multiple backends ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				ns := gatewayv1alpha2.Namespace("dummy-ns")
				udp.Spec.Rules[0].BackendRefs = make([]gatewayv1alpha2.BackendRef, 3)
				udp.Spec.Rules[0].BackendRefs[0].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[0].Name = "dummy"
				udp.Spec.Rules[0].BackendRefs[1].Namespace = &ns
				udp.Spec.Rules[0].BackendRefs[1].Name = "testservice-ok-1"
				udp.Spec.Rules[0].BackendRefs[2].Name = "testservice-ok-2"
				c.rs = []gatewayv1alpha2.UDPRoute{*udp}

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

				e1 := testutils.TestEndpoint.DeepCopy()
				e1.SetNamespace("dummy-ns")
				e1.SetName("testservice-ok-1")

				e2 := testutils.TestEndpoint.DeepCopy()
				e2.SetName("testservice-ok-2")
				e2.Subsets = []corev1.EndpointSubset{{
					Addresses: []corev1.EndpointAddress{{
						IP: "1.2.3.8",
					}},
				}}
				c.eps = []corev1.Endpoints{*e1, *e2}

			},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				// switch EDS off
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				rc, err := r.renderCluster(rs[0])
				// handle non-critical error!
				assert.NotNil(t, err, "error")
				e, ok := err.(NonCriticalRenderError)
				assert.True(t, ok, "non-critical error")
				assert.Equal(t, EndpointNotFound, e.ErrorReason, "endpoint not found error")

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
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
	})
}
