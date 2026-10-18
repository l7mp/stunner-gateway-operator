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

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func TestRenderListenerRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "udp listener ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "", lc.Addr, "bind address")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, []string{lc.Name}, lc.Servers, "servers")
			},
		},
		{
			name: "unknown proto listener errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[1]
				l.Protocol = gwapiv1.ProtocolType("dummy")

				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}
				_, err = r.renderListener(c, &l, []store.Route{}, addr, nil)
				assert.Error(t, err, "render fails")
			},
		},
		{
			name: "tcp listener ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[1]

				addr := gwAddrPort{
					addr: "5.6.7.8",
					port: 4321,
				}

				lc, err := r.renderListener(c, &l, []store.Route{}, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "5.6.7.8", lc.PublicAddr, "public-ip")
				assert.Equal(t, 4321, lc.PublicPort, "public-port")
			},
		},
		{
			name: "wrong proto errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners[0].Protocol = gwapiv1.ProtocolType("dummy")
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := r.getRoutes4Listener(gw, &l)
				assert.Len(t, rs, 1, "route found")

				addr := gwAddrPort{
					addr: "5.6.7.8",
					port: 4321,
				}

				_, err = r.renderListener(c, &l, rs, addr, nil)
				assert.Error(t, err, "wrong-proto")
			},
		},
		{
			name:  "TLS/DTLS listener ok",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(2),
					TLS:      &tls,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"),
					Port:     gwapiv1.PortNumber(3),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, "", tlsOf(lc).Cert, "cert")
				assert.Equal(t, "", tlsOf(lc).Key, "key")

				l = ls[1]
				lc, err = r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")

				l = ls[2]
				lc, err = r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-dtls", lc.Name, "name")
				assert.Equal(t, "DTLS", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1234, lc.PublicPort, "public-port")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - pqc-mode annotation ignored",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{opdefault.PQCModeAnnotationKey: "Enforced"})
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(2),
					TLS:      &tls,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"),
					Port:     gwapiv1.PortNumber(3),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				rs := []store.Route{}
				addr := gwAddrPort{addr: "1.2.3.4", port: 1234}

				// the PQC mode is a premium feature: the default renderer ignores it
				lc, err := r.renderListener(c, &ls[1], rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "", tlsOf(lc).PQCMode, "no pqc mode")
			},
		},
		{
			name:  "TLS/DTLS listener - wrong secret type ok",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Type = corev1.SecretTypeOpaque
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - secret namespace optional",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Name: gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - no secret ref invalid",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("dummy-secret"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				_, err = r.renderListener(c, &l, rs, addr, nil)
				assert.Error(t, err, "renderListener")
				assert.True(t, IsNonCriticalError(err, InvalidCertificateRef), "invalid cert ref")
			},
		},
		{
			name:  "TLS/DTLS listener - passthrough TLS is not supported",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModePassthrough
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, "", tlsOf(lc).Cert, "cert")
				assert.Equal(t, "", tlsOf(lc).Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - no secret type ok",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Type = corev1.SecretType("")
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - opaque secret type ok",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Type = corev1.SecretTypeOpaque
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - missing cert ref invalid",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Data = map[string][]byte{"tls.crt": []byte("testcert")}
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				_, err = r.renderListener(c, &l, rs, addr, nil)
				assert.Error(t, err, "renderListener")
				assert.True(t, IsNonCriticalError(err, InvalidCertificateRef), "invalid cert ref")
			},
		},
		{
			name:  "TLS/DTLS listener - missing key ref invalid",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Data = map[string][]byte{"tls.key": []byte("testkey")}
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				_, err = r.renderListener(c, &l, rs, addr, nil)
				assert.Error(t, err, "renderListener")
				assert.True(t, IsNonCriticalError(err, InvalidCertificateRef), "invalid cert ref")
			},
		},
		{
			name:  "TLS/DTLS listener - alternative cert/key data keys",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.Data = map[string][]byte{
					"cert": []byte("testcert"),
					"key":  []byte("testkey"),
				}
				c.scrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")
			},
		},
		{
			name:  "TLS/DTLS listener - multiple certificate-refs",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Name: gwapiv1.ObjectName("dummy-secret"),
					}, {
						Name: gwapiv1.ObjectName("no-key-secret"),
					}, {
						Name: gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					Port:     gwapiv1.PortNumber(1),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				s := testutils.TestSecret.DeepCopy()
				s.SetName("no-key-secret")
				s.Data = map[string][]byte{
					"dummy": []byte("dummyval"),
				}
				c.scrts = []corev1.Secret{*s, testutils.TestSecret}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				ls := gw.Spec.Listeners
				l := ls[0]

				rs := []store.Route{}
				addr := gwAddrPort{
					addr: "1.2.3.4",
					port: 1234,
				}

				lc, err := r.renderListener(c, &l, rs, addr, nil)
				assert.NoError(t, err, "renderListener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tls", lc.Name, "name")
				assert.Equal(t, "TLS", lc.Protocol, "proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(lc).Cert, "cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(lc).Key, "key")
			},
		},
	})
}

// TestGetProtocol pins the Gateway listener protocols: TURN-* feeds a TURN server over the transport
// underneath, a plain protocol an L4 server.
func TestGetProtocol(t *testing.T) {
	for proto, want := range map[string]struct {
		lproto stnrapiv2.Protocol
		stype  stnrapiv2.ServerType
	}{
		"TURN-UDP":  {stnrapiv2.ProtocolUDP, stnrapiv2.ServerTypeTURN},
		"TURN-TCP":  {stnrapiv2.ProtocolTCP, stnrapiv2.ServerTypeTURN},
		"TURN-TLS":  {stnrapiv2.ProtocolTLS, stnrapiv2.ServerTypeTURN},
		"TURN-DTLS": {stnrapiv2.ProtocolDTLS, stnrapiv2.ServerTypeTURN},
		"UDP":       {stnrapiv2.ProtocolUDP, stnrapiv2.ServerTypeL4},
		"TCP":       {stnrapiv2.ProtocolTCP, stnrapiv2.ServerTypeL4},
		"TLS":       {stnrapiv2.ProtocolTLS, stnrapiv2.ServerTypeL4},
		"DTLS":      {stnrapiv2.ProtocolDTLS, stnrapiv2.ServerTypeL4},
	} {
		lproto, stype, err := getProtocol(gwapiv1.ProtocolType(proto))
		assert.NoError(t, err, proto)
		assert.Equal(t, want.lproto, lproto, proto)
		assert.Equal(t, want.stype, stype, proto)
	}
	for _, proto := range []string{"STDIN", "FIFO", "SIP-UDP", "dummy"} {
		_, _, err := getProtocol(gwapiv1.ProtocolType(proto))
		assert.True(t, IsNonCriticalError(err, InvalidProtocol), proto)
	}
}

// tlsOf returns the TLS of a listener, empty for none.
func tlsOf(l *stnrapiv2.ListenerConfig) stnrapiv2.TLSConfig {
	if l.TLS == nil {
		return stnrapiv2.TLSConfig{}
	}
	return *l.TLS
}
