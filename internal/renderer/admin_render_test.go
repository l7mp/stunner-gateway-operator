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

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderAdminRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "admin ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, config.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, testutils.TestMetricsEndpoint, admin.MetricsEndpoint, "metrics_endpoint")
				assert.Equal(t, testutils.TestHealthCheckEndpoint, admin.HealthCheckEndpoint,
					"health-check")
			},
		},
		{
			name: "admin default ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LogLevel = nil
				w.Spec.MetricsEndpoint = nil
				w.Spec.HealthCheckEndpoint = nil
				c.cfs = []stunnerv1alpha1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, config.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, stunnerconfv1alpha1.DefaultLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, "", admin.MetricsEndpoint, "metrics_endpoint")
				assert.Equal(t, config.DefaultHealthCheckEndpoint, admin.HealthCheckEndpoint,
					"health-check default on")
			},
		},
		{
			name: "admin metricsendpoint/healthcheckendpoint ok",
			cls:  []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LogLevel = nil
				*w.Spec.MetricsEndpoint = "http://0.0.0.0:8080/metrics"
				*w.Spec.HealthCheckEndpoint = "http://0.0.0.0:8081"
				c.cfs = []stunnerv1alpha1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, config.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, stunnerconfv1alpha1.DefaultLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, "http://0.0.0.0:8080/metrics", admin.MetricsEndpoint, "Metrics_endpoint")
				assert.Equal(t, "http://0.0.0.0:8081", admin.HealthCheckEndpoint,
					"health-check default on")
			},
		},
	})
}
