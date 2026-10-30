package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"
)

func TestRenderGateway(t *testing.T) {
	r := newGatewayRenderer()

	gw := testutils.TestGw.DeepCopy()
	assert.NoError(t, r.check(nil, gw), "a Gateway with no parameters is accepted")

	gw.Spec.Infrastructure = &gwapiv1.GatewayInfrastructure{ParametersRef: &gwapiv1.LocalParametersReference{
		Group: "stunner.l7mp.io", Kind: "SIPTrunk", Name: "carriers"}}
	assert.Error(t, r.check(nil, gw), "the default renderer supports no parameters")
}
