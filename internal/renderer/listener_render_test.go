package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/event"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"
	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"
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

				_, err = r.renderListener(c, &l, rs, addr, nil)
				assert.Error(t, err, "renderListener")
				assert.False(t, IsCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, InvalidCertificateRef), "invalid certificate ref")
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
				s.Data = map[string][]byte{"tls.crt": []byte(testutils.TestCertPEM)}
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
				s.Data = map[string][]byte{"tls.key": []byte(testutils.TestKeyPEM)}
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
			name:  "TLS/DTLS listener - unparsable cert/key pair invalid",
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
				s.Data = map[string][]byte{"tls.crt": []byte("testcert"), "tls.key": []byte("testkey")}
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
					"cert": []byte(testutils.TestCertPEM),
					"key":  []byte(testutils.TestKeyPEM),
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

// tlsOf returns the TLS of a listener, empty for none.
func tlsOf(l *stnrapiv2.ListenerConfig) stnrapiv2.TLSConfig {
	if l.TLS == nil {
		return stnrapiv2.TLSConfig{}
	}
	return *l.TLS
}

func TestRenderListenerWithoutCertificate(t *testing.T) {
	mode := gwapiv1.TLSModeTerminate
	for _, tc := range []struct {
		name     string
		protocol string
		tls      *gwapiv1.ListenerTLSConfig
	}{
		{name: "TURN-TLS without a TLS block", protocol: "TURN-TLS"},
		{name: "TURN-DTLS without a TLS block", protocol: "TURN-DTLS"},
		{name: "TLS without a TLS block", protocol: "TLS"},
		{name: "TURN-TLS without certificate refs", protocol: "TURN-TLS",
			tls: &gwapiv1.ListenerTLSConfig{Mode: &mode}},
	} {
		renderTester(t, []renderTestConfig{{
			name: tc.name,
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Protocol: gwapiv1.ProtocolType(tc.protocol),
					Port:     gwapiv1.PortNumber(1),
					TLS:      tc.tls,
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
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways(gws)

				l := gws[0].Spec.Listeners[0]
				_, err = r.renderListener(c, &l, []store.Route{}, gwAddrPort{addr: "1.2.3.4", port: 1234}, nil)
				assert.True(t, IsNonCriticalError(err, InvalidCertificateRef),
					"a listener terminating TLS needs a certificate")
			},
		}})
	}
}

func TestRenderListenerProtocols(t *testing.T) {
	mode := gwapiv1.TLSModeTerminate
	ns := gwapiv1.Namespace("testnamespace")
	tls := &gwapiv1.ListenerTLSConfig{Mode: &mode, CertificateRefs: []gwapiv1.SecretObjectReference{{
		Namespace: &ns, Name: gwapiv1.ObjectName("testsecret-ok")}}}

	renderTester(t, []renderTestConfig{{
		name:  "every listener protocol",
		cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
		cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
		gws:   []gwapiv1.Gateway{testutils.TestGw},
		svcs:  []corev1.Service{testutils.TestSvc},
		dps:   []stnrgwv1.Dataplane{testutils.TestDataplane},
		scrts: []corev1.Secret{testutils.TestSecret},
		prep: func(c *renderTestConfig) {
			gw := testutils.TestGw.DeepCopy()
			gw.Spec.Listeners = []gwapiv1.Listener{
				{Name: "turn-udp", Protocol: "TURN-UDP", Port: 1},
				{Name: "turn-tcp", Protocol: "TURN-TCP", Port: 2},
				{Name: "turn-tls", Protocol: "TURN-TLS", Port: 3, TLS: tls},
				{Name: "turn-dtls", Protocol: "TURN-DTLS", Port: 4, TLS: tls},
				{Name: "udp", Protocol: "UDP", Port: 5},
				{Name: "tcp", Protocol: "TCP", Port: 6},
				{Name: "tls", Protocol: "TLS", Port: 7, TLS: tls},
				{Name: "dtls", Protocol: "DTLS", Port: 8, TLS: tls},
				{Name: "srt", Protocol: "SRT", Port: 9},
				{Name: "udp-on-the-srt-port", Protocol: "UDP", Port: 9},
				{Name: "stdin", Protocol: "STDIN", Port: 10},
				{Name: "fifo", Protocol: "FIFO", Port: 11},
				{Name: "sip-udp", Protocol: "SIP-UDP", Port: 12},
				{Name: "dummy", Protocol: "dummy", Port: 13},
			}
			c.gws = []gwapiv1.Gateway{*gw}
		},
		tester: func(t *testing.T, r *renderer) {
			gc, err := r.getGatewayClass()
			assert.NoError(t, err, "gw-class found")
			c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
			c.gwConf, err = r.getGatewayConfig4Class(c)
			assert.NoError(t, err, "gw-conf found")
			c.update = event.NewEventUpdate(0)
			c.gws.ResetGateways(r.getGateways4Class(c))

			assert.NoError(t, r.renderForGateways(c), "render success")
			conf := c.update.ConfigQueue[0]
			listeners := map[string]stnrapiv2.ListenerConfig{}
			for _, l := range conf.Listeners {
				listeners[l.Name] = l
			}

			// a TURN-* protocol is a TURN server over the transport underneath, a plain protocol
			// an L4 server
			for name, want := range map[string][2]string{
				"turn-udp":  {"UDP", "TURN"},
				"turn-tcp":  {"TCP", "TURN"},
				"turn-tls":  {"TLS", "TURN"},
				"turn-dtls": {"DTLS", "TURN"},
				"udp":       {"UDP", "L4"},
				"tcp":       {"TCP", "L4"},
				"tls":       {"TLS", "L4"},
				"dtls":      {"DTLS", "L4"},
			} {
				l, ok := listeners["testnamespace/gateway-1/"+name]
				if !assert.True(t, ok, name) {
					continue
				}
				assert.Equal(t, want[0], l.Protocol, name)
				assert.Equal(t, want[1], serverOf(conf, l).Type, name)
			}

			// an SRT listener takes its UDP port but is refused in the open-source build, internal
			// and unknown protocols are refused
			for _, name := range []string{"srt", "udp-on-the-srt-port", "stdin", "fifo", "sip-udp", "dummy"} {
				_, ok := listeners["testnamespace/gateway-1/"+name]
				assert.False(t, ok, name)
			}
			gw := c.update.UpsertQueue.Gateways.Get(store.GetNamespacedName(&testutils.TestGw)).(*gwapiv1.Gateway)
			assert.Len(t, gw.Status.Listeners, 14, "a status for every listener")
			for _, ls := range gw.Status.Listeners {
				switch ls.Name {
				case "udp-on-the-srt-port":
					cond := meta.FindStatusCondition(ls.Conditions, string(gwapiv1.ListenerConditionConflicted))
					if assert.NotNil(t, cond, ls.Name) {
						assert.Equal(t, metav1.ConditionTrue, cond.Status, ls.Name)
					}
				case "srt":
					cond := meta.FindStatusCondition(ls.Conditions, string(gwapiv1.ListenerConditionAccepted))
					if assert.NotNil(t, cond, ls.Name) {
						assert.Equal(t, metav1.ConditionFalse, cond.Status, ls.Name)
						assert.Equal(t, "SRT listeners are not supported", cond.Message, ls.Name)
					}
				case "stdin", "fifo", "sip-udp", "dummy":
					cond := meta.FindStatusCondition(ls.Conditions, string(gwapiv1.ListenerConditionAccepted))
					if assert.NotNil(t, cond, ls.Name) {
						assert.Equal(t, metav1.ConditionFalse, cond.Status, ls.Name)
						assert.Equal(t, string(gwapiv1.ListenerReasonUnsupportedProtocol), cond.Reason, ls.Name)
					}
				}
			}

			svc := c.update.UpsertQueue.Services.Get(store.GetNamespacedName(&testutils.TestGw)).(*corev1.Service)
			assert.Contains(t, svc.Spec.Ports, corev1.ServicePort{Name: "srt", Protocol: corev1.ProtocolUDP, Port: 9},
				"an SRT listener is a UDP Service port")
			udp9 := 0
			for _, sp := range svc.Spec.Ports {
				if sp.Protocol == corev1.ProtocolUDP && sp.Port == 9 {
					udp9++
				}
			}
			assert.Equal(t, 1, udp9, "the conflicted listener gets no Service port")
		},
	}})
}
