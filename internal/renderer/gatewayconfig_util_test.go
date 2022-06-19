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
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderGatewayConfigUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "no gatewayconfig errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				assert.Equal(t, gc.GetName(), "gatewayclass-ok")

				_, err = r.getGatewayConfig4Class(gc)
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref namespace errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				ns2 := gatewayv1alpha2.Namespace("dummy")
				gc.Spec.ParametersRef.Namespace = &ns2
				c.cls = []gatewayv1alpha2.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				assert.Equal(t, gc.GetName(), "gatewayclass-ok")

				_, err = r.getGatewayConfig4Class(gc)
				assert.Error(t, err, "gw-conf found")
			},
		},
		{
			name: "wrong gatewayconfig ref kind errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				gc.Spec.ParametersRef.Kind = "test"
				c.cls = []gatewayv1alpha2.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class found")
			},
		},
		{
			name: "wrong gatewayconfig ref name errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				gc := c.cls[0].DeepCopy()
				gc.Spec.ParametersRef.Name = "test"
				c.cls = []gatewayv1alpha2.GatewayClass{*gc}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				assert.Equal(t, gc.GetName(), "gatewayclass-ok")

				_, err = r.getGatewayConfig4Class(gc)
				assert.Error(t, err, "gw-conf found")
			},
		},
	})
}
