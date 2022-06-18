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

func TestRenderAuthRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "auth ok",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := gwConfig.DeepCopy()
				*w.Spec.StunnerConfig = "dummy"
				*w.Spec.Realm = "dummy"
				*w.Spec.AuthType = "longterm"
				s := "dummy"
				w.Spec.SharedSecret = &s
				c.cfs = []stunnerv1alpha1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(gwConf)

				assert.Equal(t, auth.Realm, "dummy", "realm")
				assert.Equal(t, auth.Type, "longterm", "auth-type")
				assert.Equal(t, auth.Credentials["secret"], "dummy", "secret")

			},
		},
		{
			name: "wrong auth-type errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := gwConfig.DeepCopy()
				*w.Spec.AuthType = "dummy"
				c.cfs = []stunnerv1alpha1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(gwConf)
				assert.Error(t, err, "auth-type")
			},
		},
		{
			name: "plaintext no-username errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := gwConfig.DeepCopy()
				*w.Spec.AuthType = "paintext"
				w.Spec.Username = nil
				c.cfs = []stunnerv1alpha1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(gwConf)
				assert.Error(t, err, "auth-type")
			},
		},
		{
			name: "plaintext no-password errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := gwConfig.DeepCopy()
				*w.Spec.AuthType = "paintext"
				w.Spec.Password = nil
				c.cfs = []stunnerv1alpha1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(gwConf)
				assert.Error(t, err, "auth-type")
			},
		},
		{
			name: "lonterm no-secret errs",
			cls:  []gatewayv1alpha2.GatewayClass{gwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{gwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := gwConfig.DeepCopy()
				*w.Spec.AuthType = "longterm"
				w.Spec.SharedSecret = nil
				c.cfs = []stunnerv1alpha1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(gwConf)
				assert.Error(t, err, "auth-type")
			},
		},
	})
}
