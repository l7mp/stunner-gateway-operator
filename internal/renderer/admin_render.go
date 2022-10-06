package renderer

import (
	"fmt"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) renderAdmin(c *RenderContext) (*stunnerconfv1alpha1.AdminConfig, error) {
	r.log.V(4).Info("renderAdmin", "gateway-config", store.GetObjectKey(c.gwConf))

	loglevel := stunnerconfv1alpha1.DefaultLogLevel
	if c.gwConf.Spec.LogLevel != nil {
		loglevel = *c.gwConf.Spec.LogLevel
	}

	var me string
	if c.gwConf.Spec.MetricsEndpoint != nil {
		me = *c.gwConf.Spec.MetricsEndpoint
	}

	admin := stunnerconfv1alpha1.AdminConfig{
		Name:            config.DefaultStunnerdInstanceName, // default, so that we don't reconcile it accidentally
		LogLevel:        loglevel,
		MetricsEndpoint: me,
	}

	// validate so that defaults get filled in
	if err := admin.Validate(); err != nil {
		return nil, err
	}

	r.log.V(2).Info("renderAdmin ready", "gateway-config", store.GetObjectKey(c.gwConf), "result",
		fmt.Sprintf("%#v", admin))

	return &admin, nil
}
