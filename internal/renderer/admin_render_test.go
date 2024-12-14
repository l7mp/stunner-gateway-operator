package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func TestRenderAdminRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "admin ok - legacy",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *DefaultRenderer) {
				dpMode := config.DataplaneMode
				config.DataplaneMode = config.DataplaneModeLegacy

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, admin.LogLevel, "loglevel")

				config.DataplaneMode = dpMode
			},
		},
		{
			name: "admin ok - managed",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *DefaultRenderer) {
				dpMode := config.DataplaneMode
				config.DataplaneMode = config.DataplaneModeManaged

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.dp, err = getDataplane(c)
				assert.NoError(t, err, "dataplane found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, admin.LogLevel, "loglevel")

				// disabled by default
				assert.Equal(t, "", admin.MetricsEndpoint, "metrics_endpoint")
				// enabled by default
				assert.Equal(t, opdefault.DefaultHealthCheckEndpoint, *admin.HealthCheckEndpoint,
					"health-check")

				config.DataplaneMode = dpMode
			},
		},
		{
			name: "admin metricsendpoint/healthcheckendpoint ok - managed",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LogLevel = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				dp := testutils.TestDataplane.DeepCopy()
				dp.Spec.EnableMetricsEnpoint = true
				dp.Spec.DisableHealthCheck = true
				c.dps = []stnrgwv1.Dataplane{*dp}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				dpMode := config.DataplaneMode
				config.DataplaneMode = config.DataplaneModeManaged

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.dp, err = getDataplane(c)
				assert.NoError(t, err, "dataplane found")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "renderAdmin")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, admin.Name, "name")
				assert.Equal(t, stnrconfv1.DefaultLogLevel, admin.LogLevel, "loglevel")
				assert.Equal(t, opdefault.DefaultMetricsEndpoint, admin.MetricsEndpoint, "Metrics_endpoint")
				assert.Equal(t, "", *admin.HealthCheckEndpoint, "health-check default on")

				config.DataplaneMode = dpMode
			},
		},
	})
}
