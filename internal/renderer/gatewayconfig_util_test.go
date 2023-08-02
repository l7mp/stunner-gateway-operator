package renderer

import (
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

func TestRenderGatewayConfigUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "no gatewayconfig errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				assert.Equal(t, "gatewayclass-ok", gc.GetName(), "gateway class name")

				_, err = r.getGatewayConfig4Class(c)
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref namespace errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				ns2 := gwapiv1a2.Namespace("dummy")
				gc.Spec.ParametersRef.Namespace = &ns2
				c.cls = []gwapiv1a2.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				_, err = r.getGatewayConfig4Class(c)
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref kind errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				gc.Spec.ParametersRef.Kind = "test"
				c.cls = []gwapiv1a2.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref name errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				gc.Spec.ParametersRef.Name = "test"
				c.cls = []gwapiv1a2.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				assert.Equal(t, "gatewayclass-ok", gc.GetName(), "gatewayclass name")

				_, err = r.getGatewayConfig4Class(c)
				assert.Error(t, err, "gw-conf found")
			},
		},
	})
}
