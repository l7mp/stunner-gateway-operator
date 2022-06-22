package renderer

import (
	"fmt"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

func (r *Renderer) renderAdmin(gwConf *stunnerv1alpha1.GatewayConfig) (*stunnerconfv1alpha1.AdminConfig, error) {
	r.log.V(4).Info("renderAdmin", "gateway-config", store.GetObjectKey(gwConf))

	loglevel := stunnerconfv1alpha1.DefaultLogLevel
	if gwConf.Spec.LogLevel != nil {
		loglevel = *gwConf.Spec.LogLevel
	}

	admin := stunnerconfv1alpha1.AdminConfig{
		Name:     config.DefaultStunnerdInstanceName, // default, so that we don't reconcile it accidentally
		LogLevel: loglevel,
	}

	// validate so that defaults get filled in
	if err := admin.Validate(); err != nil {
		return nil, err
	}

	r.log.V(2).Info("renderAdmin ready", "gateway-config", store.GetObjectKey(gwConf), "result",
		fmt.Sprintf("%#v", admin))

	return &admin, nil
}
