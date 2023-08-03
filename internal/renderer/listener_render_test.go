package renderer

import (
	// "context"
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

func TestRenderListenerRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "udp listener ok",
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
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
			},
		},
		{
			name: "unknown proto listener errs",
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
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[1]

				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}
				_, err = r.renderListener(gw, c.gwConf, &l, []*gwapiv1a2.UDPRoute{}, addr)
				assert.Error(t, err, "render fails")
			},
		},
		{
			name: "tcp listener ok",
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
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[2]

				addr := &gatewayAddress{
					addr: "5.6.7.8",
					port: 4321,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, []*gwapiv1a2.UDPRoute{}, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "5.6.7.8", lc.PublicAddr, "public-ip")
				assert.Equal(t, 4321, lc.PublicPort, "public-port")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
			},
		},
		{
			name: "listener defaults ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				conf := testutils.TestGwConfig.DeepCopy()
				conf.Spec.MinPort = nil
				conf.Spec.MaxPort = nil
				c.cfs = []stnrv1a1.GatewayConfig{*conf}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				addr := &gatewayAddress{
					addr: "5.6.7.8",
					port: 4321,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "5.6.7.8", lc.PublicAddr, "public-ip")
				assert.Equal(t, 4321, lc.PublicPort, "public-port")
				assert.Equal(t, stnrconfv1a1.DefaultMinRelayPort,
					lc.MinRelayPort, "min-port")
				assert.Equal(t, stnrconfv1a1.DefaultMaxRelayPort,
					lc.MaxRelayPort, "max-port")
			},
		},
		{
			name: "wrong proto errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners[0].Protocol = gwapiv1a2.ProtocolType("dummy")
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getUDPRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				addr := &gatewayAddress{
					addr: "5.6.7.8",
					port: 4321,
				}

				_, err = r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.Error(t, err, "wrong-proto")
			},
		},
		{
			name:  "TLS/DTLS listener ok",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1a2.PortNumber(1),
					Protocol: gwapiv1a2.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(2),
					TLS:      &tls,
				}, {
					Name:     gwapiv1a2.SectionName("gateway-1-listener-dtls"),
					Protocol: gwapiv1a2.ProtocolType("DTLS"),
					Port:     gwapiv1a2.PortNumber(3),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, "", lc.Cert, "cert")
				assert.Equal(t, "", lc.Key, "key")

				l = ls[1]
				lc, err = r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")

				l = ls[2]
				lc, err = r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-dtls", lc.Name, "name")
				assert.Equal(t, "DTLS", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - wrong secret type ok",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Type = corev1.SecretTypeOpaque
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - secret namespace optional",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Name: gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - no secret errs",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("dummy-secret"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "", lc.Cert, "cert")
				assert.Equal(t, "", lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - passthrough TLS is not supported",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModePassthrough
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "", lc.Cert, "cert")
				assert.Equal(t, "", lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - no secret type ok",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Type = corev1.SecretType("")
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - opaque secret type ok",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Type = corev1.SecretTypeOpaque
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - missing cert",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Data = map[string][]byte{"tls.crt": []byte("testcert")}
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "", lc.Cert, "cert")
				assert.Equal(t, "", lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - missing cert",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Data = map[string][]byte{"tls.key": []byte("testkey")}
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "", lc.Cert, "cert")
				assert.Equal(t, "", lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - alternative cert/key data keys",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Data = map[string][]byte{
					"cert": []byte("testcert"),
					"key":  []byte("testkey"),
				}
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - multiple certificate-refs",
			cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1a2.Gateway{testutils.TestGw},
			rs:    []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1b1.TLSModeTerminate
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Name: gwapiv1a2.ObjectName("dummy-secret"),
					}, {
						Name: gwapiv1a2.ObjectName("no-key-secret"),
					}, {
						Name: gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1a2.ProtocolType("TLS"),
					Port:     gwapiv1a2.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1a2.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.SetName("no-key-secret")
				s.Data = map[string][]byte{
					"dummy": []byte("dummyval"),
				}
				c.scrts = []corev1.Secret{*s, testutils.TestSecret}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []*gwapiv1a2.UDPRoute{}
				addr := &gatewayAddress{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(gw, c.gwConf, &l, rs, addr)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, lc.Cert, "cert")
				assert.Equal(t, testutils.TestKey64, lc.Key, "key")
			},
		},
	})
}
