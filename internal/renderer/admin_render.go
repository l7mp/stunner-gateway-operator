package renderer

import (
	"fmt"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	opdefault "github.com/l7mp/stunner-gateway-operator/api/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) renderAdmin(c *RenderContext) (*stnrconfv1a1.AdminConfig, error) {
	r.log.V(4).Info("renderAdmin", "gateway-config", store.GetObjectKey(c.gwConf))

	loglevel := stnrconfv1a1.DefaultLogLevel
	if c.gwConf.Spec.LogLevel != nil {
		loglevel = *c.gwConf.Spec.LogLevel
	}

	var me string
	if c.gwConf.Spec.MetricsEndpoint != nil {
		me = *c.gwConf.Spec.MetricsEndpoint
	}

	he := opdefault.DefaultHealthCheckEndpoint
	if c.gwConf.Spec.HealthCheckEndpoint != nil {
		he = *c.gwConf.Spec.HealthCheckEndpoint
	}

	admin := stnrconfv1a1.AdminConfig{
		Name:                opdefault.DefaultStunnerdInstanceName, // default, so that we don't reconcile it accidentally
		LogLevel:            loglevel,
		MetricsEndpoint:     me,
		HealthCheckEndpoint: he,
	}

	// validate so that defaults get filled in
	if err := admin.Validate(); err != nil {
		return nil, err
	}

	r.log.V(2).Info("renderAdmin ready", "gateway-config", store.GetObjectKey(c.gwConf), "result",
		fmt.Sprintf("%#v", admin))

	return &admin, nil
}
