package renderer

import (
	// "context"
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderClusterRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "cluster ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route len")

				ro := rs[0]
				p := ro.Spec.ParentRefs[0]

				accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, accepted, "route accepted")

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.testnamespace.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")
			},
		},
		{
			name: "linking to a foreign gateway errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
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
			name: "no backend errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
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

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")
			},
		},
		{
			name: "wrong backend group ignored",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
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

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")
			},
		},
		{
			name: "wrong backend kind ignored",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
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

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "udproute-wrong", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 0, "endpoints len")
			},
		},
		{
			name: "namespace ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
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

				rc, err := r.renderCluster(ro)
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.dummy.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")
			},
		},
		{
			name: "multiple backends ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
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

				rc, err := r.renderCluster(rs[0])
				assert.NoError(t, err, "render cluster")

				assert.Equal(t, "udproute-ok", rc.Name, "cluster name")
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
			},
		},
	})
}
