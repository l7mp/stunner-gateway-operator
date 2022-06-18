package renderer

import (
	// "fmt"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func (r *Renderer) renderAdmin(gwConf *stunnerv1alpha1.GatewayConfig) (*stunnerconfv1alpha1.AdminConfig, error) {
	loglevel := stunnerconfv1alpha1.DefaultLogLevel
	if gwConf.Spec.LogLevel != nil {
		loglevel = *gwConf.Spec.LogLevel
	}

	admin := stunnerconfv1alpha1.AdminConfig{
		Name:     operator.DefaultStunnerdInstanceName, // default, so that we don't reconcile it accidentally
		LogLevel: loglevel,
	}

	// validate so that defaults get filled in
	if err := admin.Validate(); err != nil {
		return nil, err
	}

	return &admin, nil
}
