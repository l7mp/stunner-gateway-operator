package renderer

import (
	// "context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderUDPRouteUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "get routes ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				ro := rs[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "udproute-ok"),
					store.GetObjectKey(ro), "route name found")
			},
		},
		{
			name: "get multiple routes ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-namespace-correct-name")
				ns := gatewayv1alpha2.Namespace("dummy-ns")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Group = nil
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Kind = nil
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Namespace = &ns
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				udp2 := testutils.TestUDPRoute.DeepCopy()
				udp2.SetName("udproute-empty-namespace-correct-name")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Group = nil
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Kind = nil
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				udp3 := testutils.TestUDPRoute.DeepCopy()
				kind := gatewayv1alpha2.Kind("dummy-kind")
				udp3.SetName("udproute-wrong-group-correct-name")
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Group = nil
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Kind = &kind
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				udp4 := testutils.TestUDPRoute.DeepCopy()
				group := gatewayv1alpha2.Group("dummy-kind")
				udp4.SetName("udproute-wrong-kind-correct-name")
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Group = &group
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Kind = nil
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				c.rs = []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute, *udp1, *udp2, *udp3, *udp4}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 2, "route found")
				keys := []string{store.GetObjectKey(rs[0]), store.GetObjectKey(rs[1])}
				assert.Contains(t, keys, fmt.Sprintf("%s/%s", testutils.TestNs, "udproute-ok"),
					"route name found")
				assert.Contains(t, keys, fmt.Sprintf("%s/%s",
					testutils.TestNs, "udproute-empty-namespace-correct-name"),
					"route name found")
			},
		},
		{
			name: "get route with listener ref ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")
				sn := gatewayv1alpha2.SectionName("gateway-1-listener-udp")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gatewayv1alpha2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "udproute-correct-listener-name"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "get route with wrong listener errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")
				sn := gatewayv1alpha2.SectionName("dummy")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gatewayv1alpha2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 0, "route not found")
			},
		},
		{
			name: "get route with multiple listener refs ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")

				udp1.Spec.CommonRouteSpec.ParentRefs =
					make([]gatewayv1alpha2.ParentReference, 3)

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
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "udproute-correct-listener-name"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "get multiple routes with listeners ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				sn1 := gatewayv1alpha2.SectionName("gateway-1-listener-udp")
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-namespace-correct-name-1")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn1

				sn2 := gatewayv1alpha2.SectionName("gateway-1-listener-tcp")
				udp2 := testutils.TestUDPRoute.DeepCopy()
				udp2.SetName("udproute-namespace-correct-name-2")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp2.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn2

				c.rs = []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute, *udp1, *udp2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners

				l := ls[0]
				rs := r.getUDPRoutes4Listener(gw, &l)

				assert.Len(t, rs, 2, "route found")
				keys := []string{store.GetObjectKey(rs[0]), store.GetObjectKey(rs[1])}
				assert.Contains(t, keys, fmt.Sprintf("%s/%s", testutils.TestNs, "udproute-ok"),
					"route name found")
				assert.Contains(t, keys, fmt.Sprintf("%s/%s",
					testutils.TestNs, "udproute-namespace-correct-name-1"),
					"route name found")

				l = ls[1]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 0, "route found")

				l = ls[2]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "udproute-namespace-correct-name-2"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "routes status ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				ro := rs[0]

				initRouteStatus(ro)
				p := ro.Spec.ParentRefs[0]
				assert.True(t, r.isParentAcceptingRoute(ro, &p, gc.GetName()))
				setRouteConditionStatus(ro, &p, config.ControllerName, true)

				assert.Len(t, ro.Status.Parents, 1, "parent status len")
				parentStatus := ro.Status.Parents[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gatewayv1alpha2.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gatewayv1alpha2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gatewayv1alpha2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gatewayv1alpha2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gatewayv1alpha2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")
			},
		},
		{
			name: "invalid routes status errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-wrong-listener-name")
				sn := gatewayv1alpha2.SectionName("dummy")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gatewayv1alpha2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")

				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route found")
				ro := rs[0]

				initRouteStatus(ro)
				p := ro.Spec.ParentRefs[0]
				assert.False(t, r.isParentAcceptingRoute(ro, &p, gc.GetName()))
				setRouteConditionStatus(ro, &p, config.ControllerName, false)

				assert.Len(t, ro.Status.Parents, 1, "parent status len")
				parentStatus := ro.Status.Parents[0]

				assert.Equal(t, p, parentStatus.ParentRef, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gatewayv1alpha2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gatewayv1alpha2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "NoMatchingListenerHostname", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gatewayv1alpha2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gatewayv1alpha2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")
			},
		},
	})
}
