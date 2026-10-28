package renderer

import (
	"fmt"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"
)

var _ configRenderer = &adminRenderer{}

type adminRenderer struct{}

func newAdminRenderer() configRenderer {
	return &adminRenderer{}
}

func (r *adminRenderer) render(c *RenderContext, _ ...any) (stnrapiv2.Config, error) {
	c.log.V(8).Info("renderAdmin (**default**)", "gateway-config", store.GetObjectKey(c.gwConf))

	loglevel := stnrapiv2.DefaultLogLevel
	if c.gwConf.Spec.LogLevel != nil {
		loglevel = *c.gwConf.Spec.LogLevel
	}

	var me string
	if c.dp != nil && c.dp.Spec.EnableMetricsEndpoint {
		me = opdefault.DefaultMetricsEndpoint
	}

	he := opdefault.DefaultHealthCheckEndpoint
	if c.dp != nil && c.dp.Spec.DisableHealthCheck {
		he = ""
	}

	offload := ""
	var offloadIntfs []string
	if c.dp != nil {
		offload = c.dp.Spec.OffloadEngine
		offloadIntfs = c.dp.Spec.OffloadInterfaces
	}

	admin := stnrapiv2.AdminConfig{
		Name:                opdefault.DefaultStunnerdInstanceName, // default, so that we don't reconcile it accidentally
		LogLevel:            loglevel,
		MetricsEndpoint:     me,
		HealthCheckEndpoint: &he,
		OffloadEngine:       offload,
		OffloadInterfaces:   offloadIntfs,
	}

	// validate so that defaults get filled in
	if err := admin.Validate(); err != nil {
		return nil, err
	}

	c.log.V(2).Info("Render admin-config ready", "gateway-config", store.GetObjectKey(c.gwConf),
		"result", fmt.Sprintf("%#v", admin))

	return &admin, nil
}
