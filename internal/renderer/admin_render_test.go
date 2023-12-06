package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func TestRenderAdminRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "admin ok",
			cls:  []gwapiv1b1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1b1.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, testutils.TestMetricsEndpoint, admin.MetricsEndpoint, "metrics_endpoint")
				assert.Equal(t, testutils.TestHealthCheckEndpoint, *admin.HealthCheckEndpoint,
					"health-check")
			},
		},
		{
			name: "admin default ok",
			cls:  []gwapiv1b1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1b1.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LogLevel = nil
				w.Spec.MetricsEndpoint = nil
				w.Spec.HealthCheckEndpoint = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, stnrconfv1.DefaultLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, "", admin.MetricsEndpoint, "metrics_endpoint")
				assert.Equal(t, opdefault.DefaultHealthCheckEndpoint, *admin.HealthCheckEndpoint,
					"health-check default on")
			},
		},
		{
			name: "admin metricsendpoint/healthcheckendpoint ok",
			cls:  []gwapiv1b1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1b1.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LogLevel = nil
				*w.Spec.MetricsEndpoint = "http://0.0.0.0:8080/metrics"
				*w.Spec.HealthCheckEndpoint = "http://0.0.0.0:8081"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, stnrconfv1.DefaultLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, "http://0.0.0.0:8080/metrics", admin.MetricsEndpoint, "Metrics_endpoint")
				assert.Equal(t, "http://0.0.0.0:8081", *admin.HealthCheckEndpoint,
					"health-check default on")
			},
		},
	})
}
