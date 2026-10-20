package renderer

import (
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
)

var _ gatewayRenderer = &defaultGatewayRenderer{}

// defaultGatewayRenderer supports no Gateway parameters: a Gateway naming any is not accepted.
type defaultGatewayRenderer struct{}

func newGatewayRenderer() gatewayRenderer {
	return &defaultGatewayRenderer{}
}

func (r *defaultGatewayRenderer) check(_ *RenderContext, gw *gwapiv1.Gateway) error {
	if gw.Spec.Infrastructure == nil || gw.Spec.Infrastructure.ParametersRef == nil {
		return nil
	}
	ref := gw.Spec.Infrastructure.ParametersRef
	return NewParametersError(gwapiv1.GatewayReasonInvalidParameters,
		"unsupported parametersRef kind %s/%s", ref.Group, ref.Kind)
}

func (r *defaultGatewayRenderer) finalize(_ *RenderContext, _ *gwapiv1.Gateway, _ *stnrapiv2.StunnerConfig) error {
	return nil
}
