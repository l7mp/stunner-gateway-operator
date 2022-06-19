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

	// "github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderListenerRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "udp listener ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				rtype := gatewayv1alpha2.AddressType("IPAddress")
				addr := gatewayv1alpha2.GatewayAddress{
					Type:  &rtype,
					Value: "1.2.3.4",
				}

				lc, err := r.renderListener(gw, gwConf, &l, rs, addr)
				assert.Equal(t, string(l.Name), lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testMaxPort), lc.MaxRelayPort, "max-port")
			},
		},
		{
			name: "unknown proto listener errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[1]

				rtype := gatewayv1alpha2.AddressType("IPAddress")
				addr := gatewayv1alpha2.GatewayAddress{
					Type:  &rtype,
					Value: "1.2.3.4",
				}

				_, err = r.renderListener(gw, gwConf, &l, []*gatewayv1alpha2.UDPRoute{}, addr)
				assert.Error(t, err, "render fails")
			},
		},
		{
			name: "tcp listener ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[2]

				rtype := gatewayv1alpha2.AddressType("IPAddress")
				addr := gatewayv1alpha2.GatewayAddress{
					Type:  &rtype,
					Value: "5.6.7.8",
				}

				lc, err := r.renderListener(gw, gwConf, &l, []*gatewayv1alpha2.UDPRoute{}, addr)
				assert.Equal(t, string(l.Name), lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "5.6.7.8", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testMaxPort), lc.MaxRelayPort, "max-port")
			},
		},
		{
			name: "listener defaults ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				conf := testGwConfig.DeepCopy()
				conf.Spec.MinPort = nil
				conf.Spec.MaxPort = nil
				c.cfs = []stunnerv1alpha1.GatewayConfig{*conf}

			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				rtype := gatewayv1alpha2.AddressType("IPAddress")
				addr := gatewayv1alpha2.GatewayAddress{
					Type:  &rtype,
					Value: "1.2.3.4",
				}

				lc, err := r.renderListener(gw, gwConf, &l, rs, addr)
				assert.Equal(t, string(l.Name), lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, stunnerconfv1alpha1.DefaultMinRelayPort,
					lc.MinRelayPort, "min-port")
				assert.Equal(t, stunnerconfv1alpha1.DefaultMaxRelayPort,
					lc.MaxRelayPort, "max-port")
			},
		},
		{
			name: "wrong proto errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				gw := testGw.DeepCopy()
				gw.Spec.Listeners[0].Protocol = gatewayv1alpha2.ProtocolType("dummy")
				c.gws = []gatewayv1alpha2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				rtype := gatewayv1alpha2.AddressType("IPAddress")
				addr := gatewayv1alpha2.GatewayAddress{
					Type:  &rtype,
					Value: "1.2.3.4",
				}

				_, err = r.renderListener(gw, gwConf, &l, rs, addr)
				assert.Error(t, err, "wrong-proto")
			},
		},
	})
}
