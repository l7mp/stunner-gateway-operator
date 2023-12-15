package renderer

import (
	// "fmt"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderGatewayClassUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "no gatewayclass errs",
			cls:  []gwapiv1.GatewayClass{},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "too many gatewayclasses errs",
			cls:  []gwapiv1.GatewayClass{},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				cls2.SetName("dummy")
				c.cls = []gwapiv1.GatewayClass{testutils.TestGwClass, *cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gcs := r.getGatewayClasses()
				assert.Len(t, gcs, 2, "gw-classes found")
			},
		},
		{
			name: "wrong controller errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				cls2.Spec.ControllerName = gwapiv1.GatewayController("dummy")
				c.cls = []gwapiv1.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "empty parametersref errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				cls2.Spec.ParametersRef = nil
				c.cls = []gwapiv1.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "invalid ref group errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				cls2.Spec.ParametersRef.Group = gwapiv1.Group("dummy")
				c.cls = []gwapiv1.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "empty ref name errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				cls2.Spec.ParametersRef.Name = ""
				c.cls = []gwapiv1.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "nil ref namespace errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				cls2.Spec.ParametersRef.Namespace = nil
				c.cls = []gwapiv1.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "empty ref namespace errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				*cls2.Spec.ParametersRef.Namespace = ""
				c.cls = []gwapiv1.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				_, err := r.getGatewayClass()
				assert.Error(t, err, "gw-class not found")
			},
		},
		{
			name: "condition status: accepted",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				setGatewayClassStatusAccepted(gc, nil)
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				assert.Equal(t, string(gwapiv1.GatewayClassConditionStatusAccepted),
					gc.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, metav1.ConditionTrue,
					gc.Status.Conditions[0].Status, "conditions status")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonAccepted),
					gc.Status.Conditions[0].Reason, "conditions reason")
				assert.Equal(t, int64(0),
					gc.Status.Conditions[0].ObservedGeneration, "conditions gen")
			},
		},
		{
			name: "condition status: re-scheduled",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				cls2 := testutils.TestGwClass.DeepCopy()
				setGatewayClassStatusAccepted(cls2, nil)
				cls2.ObjectMeta.SetGeneration(1)
				c.cls = []gwapiv1.GatewayClass{*cls2}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				setGatewayClassStatusAccepted(gc, nil)
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				assert.Equal(t, string(gwapiv1.GatewayClassConditionStatusAccepted),
					gc.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, metav1.ConditionTrue,
					gc.Status.Conditions[0].Status, "conditions status")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonAccepted),
					gc.Status.Conditions[0].Reason, "conditions reason")
				assert.Equal(t, int64(1),
					gc.Status.Conditions[0].ObservedGeneration, "conditions gen")
			},
		},
		{
			name: "condition status: invalid-params",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				setGatewayClassStatusAccepted(gc, errors.New("dummy"))
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				assert.Equal(t, string(gwapiv1.GatewayClassConditionStatusAccepted),
					gc.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, metav1.ConditionFalse,
					gc.Status.Conditions[0].Status, "conditions status")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonInvalidParameters),
					gc.Status.Conditions[0].Reason, "conditions reason")
				assert.Equal(t, int64(0),
					gc.Status.Conditions[0].ObservedGeneration, "conditions gen")
			},
		},
	})
}
