package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/event"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

func TestRenderTCPRoute(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "TCPRoute renders a TCP cluster",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			trs:  []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
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
				assert.IsType(t, &stnrgwv1.TCPRoute{}, ro, "route type")
				p := ro.GetParentRefs()[0]

				exists, accepted := r.isParentAcceptingRoute(ro, &p, "gatewayclass-ok")
				assert.True(t, exists, "route exists")
				assert.True(t, accepted, "route accepted")

				rc, _, err := r.renderCluster(nil, ro)
				assert.NoError(t, err, "renderCluster")
				assert.Equal(t, "testnamespace/tcproute-ok/tcp", rc.Name, "cluster name")
				assert.Equal(t, "TCP", rc.Protocol, "cluster protocol")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint")
			},
		},
		{
			name: "mixed UDPRoute and TCPRoute",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			trs:  []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 2, "route len")

				protos := map[string]string{}
				for _, ro := range rs {
					rc, _, err := r.renderCluster(nil, ro)
					assert.NoError(t, err, "renderCluster")
					protos[rc.Name] = rc.Protocol
				}
				assert.Equal(t, "UDP", protos["testnamespace/udproute-ok/udp"], "UDP cluster")
				assert.Equal(t, "TCP", protos["testnamespace/tcproute-ok/tcp"], "TCP cluster")

				// the TCPRoute attaches to the TCP listener via its sectionName
				gw := store.Gateways.GetFirst()
				l := gw.Spec.Listeners[1]
				lrs := r.getRoutes4Listener(gw, &l)
				assert.Len(t, lrs, 1, "listener route len")
				assert.Equal(t, "tcproute-ok", lrs[0].GetName(), "listener route name")
			},
		},
		{
			name:     "gwapiv1 TCPRoute masked by a stunner TCPRoute",
			cls:      []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:      []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			dps:      []stnrgwv1.Dataplane{testutils.TestDataplane},
			gws:      []gwapiv1.Gateway{testutils.TestGw},
			trs:      []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			trsGwAPI: []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			svcs:     []corev1.Service{testutils.TestSvc},
			prep:     func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				// the masked gwapiv1 route is skipped by allRoutes
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				assert.Same(t, store.TCPRoutes.GetObject(store.GetNamespacedName(rs[0])), rs[0],
					"route is the stunner-native one")

				masked := store.TCPRoutesGwAPI.GetAll()[0]
				assert.True(t, isRouteMasked(masked), "masked")

				// a masked route gets a Pending status
				rc := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				rc.gwConf, err = r.getGatewayConfig4Class(rc)
				assert.NoError(t, err, "gw-conf found")
				rc.update = event.NewEventUpdate(0)
				rc.gws.ResetGateways(r.getGateways4Class(rc))
				assert.NoError(t, r.renderForGateways(rc), "render success")
				status, ok := rc.update.UpsertQueue.TCPRoutesGwAPI.Get(store.GetNamespacedName(masked)).(*gwapiv1.TCPRoute)
				if !assert.True(t, ok, "route status rendered") {
					return
				}
				parents := status.Status.Parents
				assert.Len(t, parents, 1, "parent status len")
				cond := parents[0].Conditions[0]
				assert.Equal(t, string(gwapiv1.RouteConditionAccepted), cond.Type, "accepted cond")
				assert.Equal(t, metav1.ConditionFalse, cond.Status, "accepted false")
				assert.Equal(t, string(gwapiv1.RouteReasonPending), cond.Reason, "pending")
			},
		},
		{
			name: "AllowedRoutes.Kinds filters route kinds",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			trs:  []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				// the TCP listener admits only UDPRoutes
				gw.Spec.Listeners[1].AllowedRoutes = &gwapiv1.AllowedRoutes{
					Kinds: []gwapiv1.RouteGroupKind{{
						Kind: gwapiv1.Kind("UDPRoute"),
					}},
				}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gw := store.Gateways.GetFirst()

				// the UDP listener has no Kinds policy: all route kinds may attach
				lUDP := gw.Spec.Listeners[0]
				rs := r.getRoutes4Listener(gw, &lUDP)
				assert.Len(t, rs, 1, "UDP listener route len")
				assert.Equal(t, "udproute-ok", rs[0].GetName(), "UDP listener route")

				// the TCP listener rejects the TCPRoute due to the Kinds policy
				lTCP := gw.Spec.Listeners[1]
				rs = r.getRoutes4Listener(gw, &lTCP)
				assert.Len(t, rs, 0, "TCP listener route len")

				// the stunner group is accepted too
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				gw.Spec.Listeners[1].AllowedRoutes.Kinds = []gwapiv1.RouteGroupKind{{
					Group: &group,
					Kind:  gwapiv1.Kind("TCPRoute"),
				}}
				lTCP = gw.Spec.Listeners[1]
				rs = r.getRoutes4Listener(gw, &lTCP)
				assert.Len(t, rs, 1, "TCP listener route len after policy update")
			},
		},
		{
			name: "TCPRoute status resolves its refs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			trs:  []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.ClusterIP = "1.1.1.1"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				ro := rs[0]

				// render and read the status of the route
				rc := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				rc.gwConf, err = r.getGatewayConfig4Class(rc)
				assert.NoError(t, err, "gw-conf found")
				rc.update = event.NewEventUpdate(0)
				rc.gws.ResetGateways(r.getGateways4Class(rc))
				assert.NoError(t, r.renderForGateways(rc), "render success")
				ro, ok := rc.update.UpsertQueue.TCPRoutes.Get(store.GetNamespacedName(ro)).(store.Route)
				if !assert.True(t, ok, "route status rendered") {
					return
				}
				parents := ro.GetRouteStatus().Parents
				assert.Len(t, parents, 1, "parent status len")
				assert.Equal(t, "gateway-1", string(parents[0].ParentRef.Name), "parent name")

				d := meta.FindStatusCondition(parents[0].Conditions,
					string(gwapiv1.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted cond")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "accepted true")

				d = meta.FindStatusCondition(parents[0].Conditions,
					string(gwapiv1.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs cond")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "resolved true")
			},
		},
	})
}
