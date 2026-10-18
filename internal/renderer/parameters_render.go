package renderer

import (
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
)

var _ parametersRenderer = &defaultParametersRenderer{}

// defaultParametersRenderer supports no Gateway parameters: a Gateway naming any is not accepted.
type defaultParametersRenderer struct{}

func newParametersRenderer() parametersRenderer {
	return &defaultParametersRenderer{}
}

func (r *defaultParametersRenderer) check(_ *RenderContext, gw *gwapiv1.Gateway) error {
	if gw.Spec.Infrastructure == nil || gw.Spec.Infrastructure.ParametersRef == nil {
		return nil
	}
	ref := gw.Spec.Infrastructure.ParametersRef
	return NewParametersError(gwapiv1.GatewayReasonInvalidParameters,
		"unsupported parametersRef kind %s/%s", ref.Group, ref.Kind)
}

func (r *defaultParametersRenderer) finalize(_ *RenderContext, _ *gwapiv1.Gateway, _ *stnrapiv2.StunnerConfig) error {
	return nil
}
