package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderServiceUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "public-ip ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addr, err := r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "owner ref not found")
				assert.NotNil(t, addr, "public addr-port found")
				assert.Equal(t, "1.2.3.4", addr.addr, "public addr ok")
				assert.Equal(t, 1, addr.port, "public port ok")

			},
		},
		{
			name: "fallback to hostname ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Status.LoadBalancer.Ingress[0].IP = ""
				s1.Status.LoadBalancer.Ingress[0].Hostname = "dummy-hostname"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addr, err := r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "owner ref not found")
				assert.NotNil(t, addr, "public hostname found")
				assert.Equal(t, 1, addr.port, "public port ok")
				assert.Equal(t, "dummy-hostname", addr.addr, "public addr ok")
			},
		},
		{
			name: "wrong annotation name errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				delete(s1.ObjectMeta.Annotations, opdefault.RelatedGatewayAnnotationKey)
				s1.ObjectMeta.Annotations["dummy"] = "dummy"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")

			},
		},
		{
			name: "wrong annotation errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.ObjectMeta.Annotations[opdefault.RelatedGatewayAnnotationKey] = "dummy"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")
			},
		},
		{
			name: "wrong proto errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Ports[0].Protocol = corev1.ProtocolSCTP
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")

			},
		},
		{
			name: "wrong port errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Ports[0].Port = 12
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")

			},
		},
		{
			name: "no service-port stats ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Status = corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{
							IP: "1.2.3.4",
						}},
					}}
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addr, err := r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "owner ref not found")
				assert.NotNil(t, addr, "public addr-port found")
				assert.Equal(t, "1.2.3.4", addr.addr, "public addr ok")
				assert.Equal(t, 1, addr.port, "public port ok")
			},
		},
		{
			name: "multiple service-ports public-ip ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Ports[0].Protocol = corev1.ProtocolSCTP
				s1.Spec.Ports = append(s1.Spec.Ports, corev1.ServicePort{
					Name:     "tcp-ok",
					Protocol: corev1.ProtocolTCP,
					Port:     2,
				})
				s1.Status = corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{
							IP: "1.2.3.4",
							Ports: []corev1.PortStatus{{
								Port:     1,
								Protocol: corev1.ProtocolUDP,
							}},
						}, {
							IP: "5.6.7.8",
							Ports: []corev1.PortStatus{{
								Port:     1,
								Protocol: corev1.ProtocolUDP,
							}, {
								Port:     2,
								Protocol: corev1.ProtocolTCP,
							}},
						}},
					}}
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addr, err := r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "owner ref not found")
				assert.NotNil(t, addr, "public addr-port found")
				assert.Equal(t, "5.6.7.8", addr.addr, "public addr ok")
				assert.Equal(t, 2, addr.port, "public port ok")
			},
		},
		{
			name: "nodeport public-port ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Ports[0].Protocol = corev1.ProtocolSCTP
				s1.Spec.Ports = []corev1.ServicePort{{
					Name:     "udp-ok",
					Protocol: corev1.ProtocolUDP,
					Port:     1,
					NodePort: 1234,
				}}
				s1.Status = corev1.ServiceStatus{}
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addr, err := r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "owner ref not found")
				assert.NotNil(t, addr, "public addr-port found")
				// FIXME: add the public IP from nodeports!
				// assert.Equal(t, "5.6.7.8", addr.addr, "public addr ok")
				assert.Equal(t, 1234, addr.port, "public port ok")
			},
		},
		{
			name: "owner-status found",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetUID(types.UID("uid"))
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				// set owner-ref so that getPublic... finds our service
				s := types.NamespacedName{
					Namespace: testutils.TestSvc.GetNamespace(),
					Name:      testutils.TestSvc.GetName(),
				}
				svc := store.Services.GetObject(s)
				assert.NoError(t, controllerutil.SetOwnerReference(gw, svc, r.scheme), "set-owner")
				store.Services.Upsert(svc)

				addr, err := r.getPublicAddrPort4Gateway(gw)
				assert.NoError(t, err, "owner ref not found")
				assert.NotNil(t, addr, "public addr-port found")
				assert.Equal(t, "1.2.3.4", addr.addr, "public addr ok")
				assert.Equal(t, 1, addr.port, "public port ok")
			},
		},
		{
			name: "lb service - single listener",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1a2.PortNumber(1),
					Protocol: gwapiv1a2.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1a2.Gateway{*w}

			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 2, "labels len")
				lab, found := ls[opdefault.AppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.AppLabelValue, lab, "label ok")
				lab, found = ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp port")
			},
		},
		{
			name: "lb service - single listener",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1a2.PortNumber(1),
					Protocol: gwapiv1a2.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1a2.Gateway{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 2, "labels len")
				lab, found := ls[opdefault.AppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.AppLabelValue, lab, "label ok")
				lab, found = ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp port")
			},
		},
		{
			name: "lb service - single listener, no valid listener",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1a2.PortNumber(1),
					Protocol: gwapiv1a2.ProtocolType("dummy"),
				}}
				c.gws = []gwapiv1a2.Gateway{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.Nil(t, s, "svc create")
			},
		},
		{
			name: "lb service - multi-listener, single proto",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners[2].Protocol = gwapiv1a2.ProtocolType("UDP")
				c.gws = []gwapiv1a2.Gateway{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 2, "labels len")
				lab, found := ls[opdefault.AppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.AppLabelValue, lab, "label ok")
				lab, found = ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
				assert.Equal(t, string(gw.Spec.Listeners[2].Protocol), string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[2].Port), string(sp[1].Port), "sp 2 - port")
			},
		},
		{
			name: "lb service - multi-listener, multi proto",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners[0].Protocol = gwapiv1a2.ProtocolType("dummy-2")
				c.gws = []gwapiv1a2.Gateway{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 2, "labels len")
				lab, found := ls[opdefault.AppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.AppLabelValue, lab, "label ok")
				lab, found = ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[2].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[2].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb annotations",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["test"] = "testval"
				w.Spec.LoadBalancerServiceAnnotations["dummy"] = "dummyval"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 2, "labels len")
				lab, found := ls[opdefault.AppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.AppLabelValue, lab, "label ok")
				lab, found = ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 3, "annotations len")

				a, found := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["test"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "testval", a, "annotation 2 ok")

				a, found = as["dummy"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "dummyval", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb health check annotations (aws)",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 1, "labels len")
				lab, found := ls[opdefault.DefaultAppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.DefaultAppLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 3, "annotations len")

				a, found := as[opdefault.DefaultRelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/aws-load-balancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "8080", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "HTTP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")

				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, int32(8080), sp[1].Port, "sp 2 - port")
			},
		},
		{
			name: "lb service - lb health check annotations (do)",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 1, "labels len")
				lab, found := ls[opdefault.DefaultAppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.DefaultAppLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 3, "annotations len")

				a, found := as[opdefault.DefaultRelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "8080", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "HTTP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")

				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, int32(8080), sp[1].Port, "sp 2 - port")
			},
		},
		{
			name: "lb service - lb health check annotations not an int port (do)",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "eighty"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 1, "labels len")
				lab, found := ls[opdefault.DefaultAppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.DefaultAppLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 3, "annotations len")

				a, found := as[opdefault.DefaultRelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "eighty", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "HTTP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb health check annotations UDP protocol (do)",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "UDP"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 1, "labels len")
				lab, found := ls[opdefault.DefaultAppLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.DefaultAppLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 3, "annotations len")

				a, found := as[opdefault.DefaultRelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "8080", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "UDP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")

				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, int32(8080), sp[1].Port, "sp 2 - port")
			},
		},
		{
			name: "lb service - lb annotations from gwConf override from Gw",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["test"] = "testval"
				w.Spec.LoadBalancerServiceAnnotations["dummy"] = "dummyval"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann["test"] = "testval"         // same
				ann["dummy"] = "something-else" // overrride
				ann["extra"] = "extraval"       // extra
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 4, "annotations len")

				a, found := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["test"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "testval", a, "annotation 2 ok")

				a, found = as["dummy"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "something-else", a, "annotation 3 ok")

				a, found = as["extra"]
				assert.True(t, found, "annotation 4 found")
				assert.Equal(t, "extraval", a, "annotation 4 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - default svc type",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
			},
		},
		{
			name: "lb service - svc type ClusterIP from gwConf",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeClusterIP, spec.Type, "svc type")
			},
		},
		{
			name: "lb service - svc type NodePort from gwConf",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "NodePort"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeNodePort, spec.Type, "svc type")
			},
		},
		{
			name: "lb service - svc type from svc annotation",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeClusterIP, spec.Type, "svc type")
			},
		},
		{
			name: "lb service - nodeport svc override gwConf from svc annotation",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann[opdefault.ServiceTypeAnnotationKey] = "NodePort"
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeNodePort, spec.Type, "svc type")
			},
		},
		{
			name: "lb service - svc type NodePort from gw annotation",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann[opdefault.ServiceTypeAnnotationKey] = "NodePort"
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s := createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeNodePort, spec.Type, "svc type")
			},
		},
	})
}
