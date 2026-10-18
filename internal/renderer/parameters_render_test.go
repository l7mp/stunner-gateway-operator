package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
)

func TestRenderParameters(t *testing.T) {
	r := newParametersRenderer()

	gw := testutils.TestGw.DeepCopy()
	assert.NoError(t, r.check(nil, gw), "a Gateway with no parameters is accepted")

	gw.Spec.Infrastructure = &gwapiv1.GatewayInfrastructure{ParametersRef: &gwapiv1.LocalParametersReference{
		Group: "stunner.l7mp.io", Kind: "SIPTrunk", Name: "carriers"}}
	err := r.check(nil, gw)
	assert.Error(t, err, "the default renderer supports no parameters")

	// the Gateway is not accepted for its parameters
	initGatewayStatus(gw, err)
	cond := meta.FindStatusCondition(gw.Status.Conditions, string(gwapiv1.GatewayConditionAccepted))
	assert.NotNil(t, cond, "accepted cond")
	assert.Equal(t, metav1.ConditionFalse, cond.Status, "not accepted")
	assert.Equal(t, string(gwapiv1.GatewayReasonInvalidParameters), cond.Reason, "reason")
	assert.Contains(t, cond.Message, "SIPTrunk", "message")
}
