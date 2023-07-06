package renderer

import (
	// "context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderUDPRouteUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "get routes",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				ro := rs[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-ok"),
					store.GetObjectKey(ro), "route name found")
			},
		},
		{
			name: "get multiple routes",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-namespace-correct-name")
				ns := gwapiv1a2.Namespace("dummy-ns")
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
				kind := gwapiv1a2.Kind("dummy-kind")
				udp3.SetName("udproute-wrong-group-correct-name")
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Group = nil
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Kind = &kind
				udp3.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				udp4 := testutils.TestUDPRoute.DeepCopy()
				group := gwapiv1a2.Group("dummy-kind")
				udp4.SetName("udproute-wrong-kind-correct-name")
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Group = &group
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Kind = nil
				udp4.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"

				c.rs = []gwapiv1a2.UDPRoute{testutils.TestUDPRoute, *udp1, *udp2, *udp3, *udp4}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 2, "route found")
				keys := []string{store.GetObjectKey(rs[0]), store.GetObjectKey(rs[1])}
				assert.Contains(t, keys, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-ok"),
					"route name found")
				assert.Contains(t, keys, fmt.Sprintf("%s/%s",
					testutils.TestNsName, "udproute-empty-namespace-correct-name"),
					"route name found")
			},
		},
		{
			name: "get multiple routes with route attachment policy All",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				// allow from only one namespace
				fromNamespaces := gwapiv1b1.NamespacesFromAll
				routeNamespaces := gwapiv1a2.RouteNamespaces{
					From: &fromNamespaces,
				}
				allowedRoutes := gwapiv1a2.AllowedRoutes{
					Namespaces: &routeNamespaces,
				}
				gw.Spec.Listeners[0].AllowedRoutes = &allowedRoutes
				c.gws = []gwapiv1a2.Gateway{*gw}

				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-testnamespace")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = nil

				udp2 := testutils.TestUDPRoute.DeepCopy()
				udp2.SetName("udproute-dummy-namespace")
				udp2.SetNamespace("dummy-namespace")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp2.Spec.CommonRouteSpec.ParentRefs[0].SectionName = nil

				c.rs = []gwapiv1a2.UDPRoute{*udp1, *udp2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners

				l := ls[0]
				rs := r.getUDPRoutes4Listener(gw, &l)

				// listener accepts both routes: attachment policy is All
				assert.Len(t, rs, 2, "route found")
				keys := []string{store.GetObjectKey(rs[0]), store.GetObjectKey(rs[1])}
				assert.Contains(t, keys, "testnamespace/udproute-testnamespace",
					"route name found")
				assert.Contains(t, keys, "dummy-namespace/udproute-dummy-namespace",
					"route name found")

				l = ls[1]
				rs = r.getUDPRoutes4Listener(gw, &l)
				// listener rejects route from different namespace as attachment policy is Same
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "testnamespace/udproute-testnamespace", store.GetObjectKey(rs[0]),
					"route name found")

				l = ls[2]
				rs = r.getUDPRoutes4Listener(gw, &l)
				// listener rejects route from different namespace as attachment policy is Same
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "testnamespace/udproute-testnamespace", store.GetObjectKey(rs[0]),
					"route name found")
			},
		},
		{
			name: "get multiple routes with route attachment policy Selector",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			nss:  []corev1.Namespace{testutils.TestNs},
			prep: func(c *renderTestConfig) {
				// add dummy-namespace
				ns := testutils.TestNs.DeepCopy()
				ns.SetName("dummy-namespace")
				ns.SetLabels(map[string]string{"dummy-label": "dummy-value"})
				c.nss = []corev1.Namespace{testutils.TestNs, *ns}

				gw := testutils.TestGw.DeepCopy()
				// allow from only from testnamespace
				fromNamespaces := gwapiv1b1.NamespacesFromSelector
				routeNamespaces1 := gwapiv1a2.RouteNamespaces{
					From: &fromNamespaces,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{testutils.TestLabelName: testutils.TestLabelValue},
					},
				}
				allowedRoutes1 := gwapiv1a2.AllowedRoutes{
					Namespaces: &routeNamespaces1,
				}
				gw.Spec.Listeners[0].AllowedRoutes = &allowedRoutes1

				// allow from only from dummy-namespace
				routeNamespaces2 := gwapiv1a2.RouteNamespaces{
					From: &fromNamespaces,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"dummy-label": "dummy-value"},
					},
				}
				allowedRoutes2 := gwapiv1a2.AllowedRoutes{
					Namespaces: &routeNamespaces2,
				}
				gw.Spec.Listeners[2].AllowedRoutes = &allowedRoutes2
				c.gws = []gwapiv1a2.Gateway{*gw}

				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-testnamespace")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = nil

				udp2 := testutils.TestUDPRoute.DeepCopy()
				udp2.SetName("udproute-dummy-namespace")
				udp2.SetNamespace("dummy-namespace")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp2.Spec.CommonRouteSpec.ParentRefs[0].SectionName = nil

				c.rs = []gwapiv1a2.UDPRoute{*udp1, *udp2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners

				l := ls[0]
				rs := r.getUDPRoutes4Listener(gw, &l)

				// listener accepts only one route: attachment policy is Selector
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "testnamespace/udproute-testnamespace", store.GetObjectKey(rs[0]),
					"route name found")

				l = ls[1]
				rs = r.getUDPRoutes4Listener(gw, &l)
				// listener rejects route from different namespace as attachment policy is Same
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "testnamespace/udproute-testnamespace", store.GetObjectKey(rs[0]),
					"route name found")

				l = ls[2]
				rs = r.getUDPRoutes4Listener(gw, &l)
				// listener accepts only one route: attachment policy is Selector
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "dummy-namespace/udproute-dummy-namespace", store.GetObjectKey(rs[0]),
					"route name found")
			},
		},
		{
			name: "get route with listener ref",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")
				sn := gwapiv1a2.SectionName("gateway-1-listener-udp")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gwapiv1a2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-correct-listener-name"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "get route with wrong listener errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")
				sn := gwapiv1a2.SectionName("dummy")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gwapiv1a2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 0, "route not found")
			},
		},
		{
			name: "get route with multiple listener refs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-correct-listener-name")

				udp1.Spec.CommonRouteSpec.ParentRefs =
					make([]gwapiv1a2.ParentReference, 3)

				sn1 := gwapiv1a2.SectionName("gateway-1-listener-udp")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn1

				sn2 := gwapiv1a2.SectionName("gateway-1-listener-tcp")
				udp1.Spec.CommonRouteSpec.ParentRefs[1].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[1].SectionName = &sn2

				sn3 := gwapiv1a2.SectionName("dummy")
				udp1.Spec.CommonRouteSpec.ParentRefs[2].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[2].SectionName = &sn3

				c.rs = []gwapiv1a2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-correct-listener-name"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "get multiple routes with listeners",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				sn1 := gwapiv1a2.SectionName("gateway-1-listener-udp")
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-namespace-correct-name-1")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn1

				sn2 := gwapiv1a2.SectionName("gateway-1-listener-tcp")
				udp2 := testutils.TestUDPRoute.DeepCopy()
				udp2.SetName("udproute-namespace-correct-name-2")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp2.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn2

				c.rs = []gwapiv1a2.UDPRoute{testutils.TestUDPRoute, *udp1, *udp2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners

				l := ls[0]
				rs := r.getUDPRoutes4Listener(gw, &l)

				assert.Len(t, rs, 2, "route found")
				keys := []string{store.GetObjectKey(rs[0]), store.GetObjectKey(rs[1])}
				assert.Contains(t, keys, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-ok"),
					"route name found")
				assert.Contains(t, keys, fmt.Sprintf("%s/%s",
					testutils.TestNsName, "udproute-namespace-correct-name-1"),
					"route name found")

				l = ls[1]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 0, "route found")

				l = ls[2]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-namespace-correct-name-2"),
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "get multiple routes with listeners and route attachment policy All",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				// allow from only one namespace
				fromNamespaces := gwapiv1b1.NamespacesFromAll
				routeNamespaces := gwapiv1a2.RouteNamespaces{
					From: &fromNamespaces,
				}
				allowedRoutes := gwapiv1a2.AllowedRoutes{
					Namespaces: &routeNamespaces,
				}
				gw.Spec.Listeners[0] = gwapiv1a2.Listener{
					Name:          gwapiv1a2.SectionName("gateway-1-listener-udp"),
					Port:          gwapiv1a2.PortNumber(1),
					Protocol:      gwapiv1a2.ProtocolType("UDP"),
					AllowedRoutes: &allowedRoutes,
				}
				c.gws = []gwapiv1a2.Gateway{*gw}

				sn1 := gwapiv1a2.SectionName("gateway-1-listener-udp")
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-dummy-namespace-listener-udp")
				udp1.SetNamespace("dummy-namespace")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn1

				sn2 := gwapiv1a2.SectionName("gateway-1-listener-tcp")
				udp2 := testutils.TestUDPRoute.DeepCopy()
				udp2.SetName("udproute-dummy-namespace-listener-tcp")
				udp2.SetNamespace("dummy-namespace")
				udp2.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp2.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn2

				c.rs = []gwapiv1a2.UDPRoute{*udp1, *udp2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners

				l := ls[0]
				rs := r.getUDPRoutes4Listener(gw, &l)

				// gw accepts route from other namespace as attachment policy is All
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "dummy-namespace/udproute-dummy-namespace-listener-udp",
					store.GetObjectKey(rs[0]),
					"route found")

				l = ls[1]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 0, "route found")

				l = ls[2]
				rs = r.getUDPRoutes4Listener(gw, &l)
				// gw rejects route from other namespace as attachment policy is Same
				assert.Len(t, rs, 0, "route found")
			},
		},
		{
			name: "get multiple routes with listeners and route attachment policy Selector",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// add dummy-namespace
				ns := testutils.TestNs.DeepCopy()
				ns.SetName("dummy-namespace")
				ns.SetLabels(map[string]string{"dummy-label": "dummy-value"})
				c.nss = []corev1.Namespace{testutils.TestNs, *ns}

				gw := testutils.TestGw.DeepCopy()
				// allow from both namespaces
				fromNamespaces := gwapiv1b1.NamespacesFromSelector
				routeNamespaces := gwapiv1a2.RouteNamespaces{
					From: &fromNamespaces,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"dummy-label": "dummy-value"},
					},
				}
				allowedRoutes := gwapiv1a2.AllowedRoutes{
					Namespaces: &routeNamespaces,
				}
				gw.Spec.Listeners[0] = gwapiv1a2.Listener{
					Name:          gwapiv1a2.SectionName("gateway-1-listener-udp"),
					Port:          gwapiv1a2.PortNumber(1),
					Protocol:      gwapiv1a2.ProtocolType("UDP"),
					AllowedRoutes: &allowedRoutes,
				}
				c.gws = []gwapiv1a2.Gateway{*gw}

				sn := gwapiv1a2.SectionName("gateway-1-listener-tcp")
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-testnamespace")
				udp1.Spec.CommonRouteSpec.ParentRefs = []gwapiv1a2.ParentReference{
					{
						Name:        "gateway-1",
						SectionName: &testutils.TestSectionName,
					},
					{
						Name:        "gateway-1",
						SectionName: &sn,
					},
				}

				udp2 := testutils.TestUDPRoute.DeepCopy()
				udp2.SetName("udproute-dummy-namespace-listener-udp")
				udp2.SetNamespace("dummy-namespace")
				udp2.Spec.CommonRouteSpec.ParentRefs = []gwapiv1a2.ParentReference{
					{
						Name:        "gateway-1",
						SectionName: &testutils.TestSectionName,
					},
					{
						Name:        "gateway-1",
						SectionName: &sn,
					},
				}
				c.rs = []gwapiv1a2.UDPRoute{*udp1, *udp2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				ls := gw.Spec.Listeners

				l := ls[0]
				rs := r.getUDPRoutes4Listener(gw, &l)

				// gw accepts route from other namespace as attachment policy is All
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "dummy-namespace/udproute-dummy-namespace-listener-udp",
					store.GetObjectKey(rs[0]), "route found")

				l = ls[1]
				rs = r.getUDPRoutes4Listener(gw, &l)
				// does not match sectionname
				assert.Len(t, rs, 0, "route found")

				l = ls[2]
				rs = r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")
				assert.Equal(t, "testnamespace/udproute-testnamespace",
					store.GetObjectKey(rs[0]), "route name found")
			},
		},
		{
			name: "valid routes - status",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

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
				setRouteConditionStatus(ro, &p, config.ControllerName, true, nil)

				assert.Len(t, ro.Status.Parents, 1, "parent status len")
				parentStatus := ro.Status.Parents[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1a2.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")
			},
		},
		{
			name: "invalid routes - status",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-wrong-listener-name")
				sn := gwapiv1a2.SectionName("dummy")
				udp1.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				udp1.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				c.rs = []gwapiv1a2.UDPRoute{*udp1}
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
				setRouteConditionStatus(ro, &p, config.ControllerName, false, nil)

				assert.Len(t, ro.Status.Parents, 1, "parent status len")
				parentStatus := ro.Status.Parents[0]

				assert.Equal(t, p, parentStatus.ParentRef, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "NotAllowedByListeners", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")
			},
		},
		{
			name: "valid cross-namespace route - status",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				// allow from only one namespace
				fromNamespaces := gwapiv1b1.NamespacesFromAll
				routeNamespaces := gwapiv1a2.RouteNamespaces{
					From: &fromNamespaces,
				}
				allowedRoutes := gwapiv1a2.AllowedRoutes{
					Namespaces: &routeNamespaces,
				}
				gw.Spec.Listeners[0] = gwapiv1a2.Listener{
					Name:          gwapiv1a2.SectionName("gateway-1-listener-udp"),
					Port:          gwapiv1a2.PortNumber(1),
					Protocol:      gwapiv1a2.ProtocolType("UDP"),
					AllowedRoutes: &allowedRoutes,
				}
				c.gws = []gwapiv1a2.Gateway{*gw}
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-cross-namespace")
				udp1.SetNamespace("dummy-namespace")
				sn := gwapiv1a2.Namespace("testnamespace")
				udp1.Spec.ParentRefs[0] = gwapiv1a2.ParentReference{
					Name:      "gateway-1",
					Namespace: &sn,
				}
				c.rs = []gwapiv1a2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")

				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route found")
				ro := rs[0]

				initRouteStatus(ro)
				p := ro.Spec.ParentRefs[0]
				accepted := r.isParentAcceptingRoute(ro, &p, gc.GetName())
				assert.True(t, accepted, "accepted")
				setRouteConditionStatus(ro, &p, config.ControllerName, accepted, nil)

				assert.Len(t, ro.Status.Parents, 1, "parent status len")
				parentStatus := ro.Status.Parents[0]

				assert.Equal(t, p, parentStatus.ParentRef, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")
			},
		},
		{
			name: "invalid cross-namespace route - status",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				udp1 := testutils.TestUDPRoute.DeepCopy()
				udp1.SetName("udproute-cross-namespace")
				udp1.SetNamespace("dummy-namespace")
				sn := gwapiv1a2.Namespace("testnamespace")
				udp1.Spec.ParentRefs[0] = gwapiv1a2.ParentReference{
					Name:      "gateway-1",
					Namespace: &sn,
				}
				c.rs = []gwapiv1a2.UDPRoute{*udp1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")

				rs := store.UDPRoutes.GetAll()
				assert.Len(t, rs, 1, "route found")
				ro := rs[0]

				initRouteStatus(ro)
				p := ro.Spec.ParentRefs[0]
				accepted := r.isParentAcceptingRoute(ro, &p, gc.GetName())
				assert.False(t, accepted, "accepted")
				setRouteConditionStatus(ro, &p, config.ControllerName, accepted, nil)

				assert.Len(t, ro.Status.Parents, 1, "parent status len")
				parentStatus := ro.Status.Parents[0]

				assert.Equal(t, p, parentStatus.ParentRef, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "NotAllowedByListeners", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")
			},
		},
	})
}
