package renderer

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *DefaultRenderer) getGatewayConfig4Class(c *RenderContext) (*stnrgwv1.GatewayConfig, error) {
	// r.log.V(4).Info("getGatewayConfig4Class", "gateway-class", store.GetObjectKey(c.gc))

	// ref already checked
	ref := c.gc.Spec.ParametersRef

	gwConfName := types.NamespacedName{
		Namespace: string(*ref.Namespace), // this should already be validated
		Name:      ref.Name,
	}

	gwConf := store.GatewayConfigs.GetObject(gwConfName)
	if gwConf == nil {
		return nil, fmt.Errorf("No GatewayConfig found for name: %s",
			gwConfName.String())
	}

	r.log.V(4).Info("Finished searching gateways for gateway-class", "gateway-class", store.GetObjectKey(c.gc),
		"result", store.GetObjectKey(gwConf))

	return gwConf, nil
}
