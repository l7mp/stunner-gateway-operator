package renderer

import (
	// "context"
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderServiceUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "public-ip ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
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
			name: "wrong annotation name errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				delete(s1.ObjectMeta.Annotations, config.GatewayAddressAnnotationKey)
				s1.ObjectMeta.Annotations["dummy"] = "dummy"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")

			},
		},
		{
			name: "wrong annotation errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.ObjectMeta.Annotations[config.GatewayAddressAnnotationKey] = "dummy"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")
			},
		},
		{
			name: "wrong proto errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Ports[0].Protocol = corev1.ProtocolSCTP
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")

			},
		},
		{
			name: "wrong port errs",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Ports[0].Port = 12
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddrPort4Gateway(gw)
				assert.Error(t, err, "public addr-port found")

			},
		},
		{
			name: "no service-port stats ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
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
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
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
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
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
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
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
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
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
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
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
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetUID(types.UID("uid"))
				c.gws = []gatewayv1alpha2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				_, err = r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
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
	})
}
