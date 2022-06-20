package renderer

import (
	// "context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderUDPRouteUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "get routes ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				ro := rs[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-ok"),
					store.GetObjectKey(ro), "route name found")
			},
		},
		{
			name: "get multiple routes ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testUDPRoute.DeepCopy()
				udp1.SetName("udproute-namespace-correct-name")
				ns := gatewayv1alpha2.Namespace("dummy-ns")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Group = nil
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Kind = nil
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Namespace = &ns
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				udp2 := testUDPRoute.DeepCopy()
				udp2.SetName("udproute-empty-namespace-correct-name")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Group = nil
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Kind = nil
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				udp3 := testUDPRoute.DeepCopy()
				kind := gatewayv1alpha2.Kind("dummy-kind")
				udp3.SetName("udproute-wrong-group-correct-name")
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Group = nil
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Kind = &kind
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				udp4 := testUDPRoute.DeepCopy()
				group := gatewayv1alpha2.Group("dummy-kind")
				udp4.SetName("udproute-wrong-kind-correct-name")
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Group = &group
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Kind = nil
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				c.rs = []gatewayv1alpha2.UDPRoute{testUDPRoute, *udp1, *udp2, *udp3, *udp4}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 2, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-ok"),
					store.GetObjectKey(rs[0]), "route name found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-empty-namespace-correct-name"),
					store.GetObjectKey(rs[1]), "route name found")
			},
		},
		{
			name: "get route with listener ref ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")
				sn := gatewayv1alpha2.SectionName("gateway-1-listener-udp")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gatewayv1alpha2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-correct-listener-name"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "get route with wrong listener errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")
				sn := gatewayv1alpha2.SectionName("dummy")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gatewayv1alpha2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 0, "route not found")
			},
		},
		{
			name: "get route with multiple listener refs ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")

				udp1.Spec.CommonRouteSpec.ParentRefs =
					make([]gatewayv1alpha2.ParentRef, 3)

				sn1 := gatewayv1alpha2.SectionName("gateway-1-listener-udp")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn1

				sn2 := gatewayv1alpha2.SectionName("gateway-1-listener-tcp")
				udp1.Spec.CommonRouteSpec.ParentRefs[1].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[1].SectionName = &sn2

				sn3 := gatewayv1alpha2.SectionName("dummy")
				udp1.Spec.CommonRouteSpec.ParentRefs[2].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[2].SectionName = &sn3

				c.rs = []gatewayv1alpha2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-correct-listener-name"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "get multiple routes with listeners ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				sn1 := gatewayv1alpha2.SectionName("gateway-1-listener-udp")
				udp1 := testUDPRoute.DeepCopy()
				udp1.SetName("udproute-namespace-correct-name-1")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn1

				sn2 := gatewayv1alpha2.SectionName("gateway-1-listener-tcp")
				udp2 := testUDPRoute.DeepCopy()
				udp2.SetName("udproute-namespace-correct-name-2")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp2.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn2

				c.rs = []gatewayv1alpha2.UDPRoute{testUDPRoute, *udp1, *udp2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners

				l := ls[0]
				rs := r.getUDPRoutes4Listener(gw, &l)

				assert.Len(t, rs, 2, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-ok"),
					store.GetObjectKey(rs[0]), "route name found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-namespace-correct-name-1"),
					store.GetObjectKey(rs[1]), "route name found")

				l = ls[1]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 0, "route found")

				l = ls[2]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "udproute-namespace-correct-name-2"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
	})
}
