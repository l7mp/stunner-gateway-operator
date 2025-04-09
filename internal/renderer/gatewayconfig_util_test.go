package renderer

import (
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderGatewayConfigUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "no gatewayconfig errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				assert.Equal(t, "gatewayclass-ok", gc.GetName(), "gateway class name")

				_, err = r.getGatewayConfig4Class(c)
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref namespace errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				ns2 := gwapiv1.Namespace("dummy")
				gc.Spec.ParametersRef.Namespace = &ns2
				c.cls = []gwapiv1.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				_, err = r.getGatewayConfig4Class(c)
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref kind errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				gc.Spec.ParametersRef.Kind = "test"
				c.cls = []gwapiv1.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref name errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				gc.Spec.ParametersRef.Name = "test"
				c.cls = []gwapiv1.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				assert.Equal(t, "gatewayclass-ok", gc.GetName(), "gatewayclass name")

				_, err = r.getGatewayConfig4Class(c)
				assert.Error(t, err, "gw-conf found")
			},
		},
	})
}
