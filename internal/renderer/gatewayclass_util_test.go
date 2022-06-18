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

func TestRenderGatewayClassUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "no gatewayclass errs",
			cls:  []gatewayv1alpha2.GatewayClass{},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "too many gatewayclasses errs",
			cls:  []gatewayv1alpha2.GatewayClass{},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := gwClass.DeepCopy()
				cls2.SetName("dummy")
				c.cls = []gatewayv1alpha2.GatewayClass{gwClass, *cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "wrong controller errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := gwClass.DeepCopy()
				cls2.Spec.ControllerName = gatewayv1alpha2.GatewayController("dummy")
				c.cls = []gatewayv1alpha2.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "empty parametersref errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := gwClass.DeepCopy()
				cls2.Spec.ParametersRef = nil
				c.cls = []gatewayv1alpha2.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "invalid ref group errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := gwClass.DeepCopy()
				cls2.Spec.ParametersRef.Group = gatewayv1alpha2.Group("dummy")
				c.cls = []gatewayv1alpha2.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "empty ref name errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := gwClass.DeepCopy()
				cls2.Spec.ParametersRef.Name = ""
				c.cls = []gatewayv1alpha2.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "nil ref namespace errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := gwClass.DeepCopy()
				cls2.Spec.ParametersRef.Namespace = nil
				c.cls = []gatewayv1alpha2.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "empty ref namespace errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := gwClass.DeepCopy()
				*cls2.Spec.ParametersRef.Namespace = ""
				c.cls = []gatewayv1alpha2.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
	})
}
