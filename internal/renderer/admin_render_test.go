package renderer

import (
	// "context"
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderAdminRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "admin ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
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
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
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
				assert.Equal(t, stnrconfv1a1.DefaultLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, "", admin.MetricsEndpoint, "metrics_endpoint")
				assert.Equal(t, opdefault.DefaultHealthCheckEndpoint, *admin.HealthCheckEndpoint,
					"health-check default on")
			},
		},
		{
			name: "admin metricsendpoint/healthcheckendpoint ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
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
				assert.Equal(t, stnrconfv1a1.DefaultLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, "http://0.0.0.0:8080/metrics", admin.MetricsEndpoint, "Metrics_endpoint")
				assert.Equal(t, "http://0.0.0.0:8081", *admin.HealthCheckEndpoint,
					"health-check default on")
			},
		},
	})
}
