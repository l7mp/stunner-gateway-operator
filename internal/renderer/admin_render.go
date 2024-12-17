package renderer

import (
	"fmt"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

var _ configRenderer = &adminRenderer{}

type adminRenderer struct{}

func newAdminRenderer() configRenderer {
	return &adminRenderer{}
}

func (r *adminRenderer) render(c *RenderContext) (stnrconfv1.Config, error) {
	c.log.V(8).Info("renderAdmin (**default**)", "gateway-config", store.GetObjectKey(c.gwConf))

	loglevel := stnrconfv1.DefaultLogLevel
	if c.gwConf.Spec.LogLevel != nil {
		loglevel = *c.gwConf.Spec.LogLevel
	}

	var me string
	if config.DataplaneMode == config.DataplaneModeManaged && c.dp != nil && c.dp.Spec.EnableMetricsEnpoint {
		me = opdefault.DefaultMetricsEndpoint
	}

	he := opdefault.DefaultHealthCheckEndpoint
	if config.DataplaneMode == config.DataplaneModeManaged && c.dp != nil && c.dp.Spec.DisableHealthCheck {
		he = ""
	}

	admin := stnrconfv1.AdminConfig{
		Name:                opdefault.DefaultStunnerdInstanceName, // default, so that we don't reconcile it accidentally
		LogLevel:            loglevel,
		MetricsEndpoint:     me,
		HealthCheckEndpoint: &he,
	}

	// validate so that defaults get filled in
	if err := admin.Validate(); err != nil {
		return nil, err
	}

	c.log.V(2).Info("Render admin-config ready", "gateway-config", store.GetObjectKey(c.gwConf),
		"result", fmt.Sprintf("%#v", admin))

	return &admin, nil
}
