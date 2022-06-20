package renderer

import (
	// "context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderGatewayUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "wrong gatewayclassname errs",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				gw := testGw.DeepCopy()
				gw.Spec.GatewayClassName = "dummy"
				c.gws = []gatewayv1alpha2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 0, "gw found")
			},
		},
		{
			name: "multiple gateways ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				gw := testGw.DeepCopy()
				gw.ObjectMeta.SetName("dummy")
				gw.ObjectMeta.SetGeneration(4)
				c.gws = []gatewayv1alpha2.Gateway{*gw, testGw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 2, "gw found")

				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "dummy"),
					store.GetObjectKey(gws[0]), "gw 1 name found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gws[1]), "gw 2 name found")
			},
		},
		{
			name: "gateway status ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				setGatewayStatusScheduled(gw, "dummy")
				setGatewayStatusReady(gw, "dummy")
				assert.Len(t, gw.Status.Conditions, 2, "conditions num")
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled),
					gw.Status.Conditions[0].Type, "conditions sched")
				assert.Equal(t, int64(0), gw.Status.Conditions[0].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionReady),
					gw.Status.Conditions[1].Type, "conditions ready")
				assert.Equal(t, int64(0), gw.Status.Conditions[1].ObservedGeneration,
					"conditions gen")
			},
		},
		{
			name: "gateway rescheduled/re-ready status ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				gw := testGw.DeepCopy()
				setGatewayStatusScheduled(gw, "dummy")
				setGatewayStatusReady(gw, "dummy")
				gw.ObjectMeta.SetGeneration(1)
				c.gws = []gatewayv1alpha2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				setGatewayStatusScheduled(gw, "dummy")
				setGatewayStatusReady(gw, "dummy")
				assert.Len(t, gw.Status.Conditions, 2, "conditions num")
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionScheduled),
					gw.Status.Conditions[0].Type, "conditions sched")
				assert.Equal(t, int64(1),
					gw.Status.Conditions[0].ObservedGeneration, "conditions gen")
				assert.Equal(t, string(gatewayv1alpha2.GatewayConditionReady),
					gw.Status.Conditions[1].Type, "conditions ready")
				assert.Equal(t, int64(1),
					gw.Status.Conditions[1].ObservedGeneration, "conditions gen")
			},
		},
		{
			name: "lisener status ok",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {
				// u := testUDPRoute.DeepCopy()
				// u.ObjectMeta.SetName("tcp")
				// u.Spec.
				// 	c.rs = []gatewayv1alpha2.UDPRoute{*u, testUDPRoute}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")

				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(gc)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, store.GetObjectKey(gw), fmt.Sprintf("%s/%s",
					testNs, "gateway-1"), "gw name found")

				rtype := gatewayv1alpha2.AddressType("IPAddress")
				addr := gatewayv1alpha2.GatewayAddress{
					Type:  &rtype,
					Value: "1.2.3.4",
				}

				setGatewayStatusScheduled(gw, r.op.GetControllerName())
				ready := true
				for j := range gw.Spec.Listeners {
					l := gw.Spec.Listeners[j]
					_, err := r.renderListener(gw, gwConf, &l,
						[]*gatewayv1alpha2.UDPRoute{}, addr)

					if err != nil {
						setListenerStatus(gw, &l, false, ready, 0)
						continue
					}

					setListenerStatus(gw, &l, true, ready, 2)
				}

				// listeners[0]: ok
				assert.Len(t, gw.Status.Listeners, 3, "conditions num")
				d := meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gatewayv1alpha2.ListenerConditionDetached))
				assert.NotNil(t, d, "detached found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionDetached), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonAttached), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gatewayv1alpha2.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gatewayv1alpha2.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonReady),
					d.Reason, "reason")

				// listeners[1]: detached
				assert.Len(t, gw.Status.Listeners, 3, "conditions num")
				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gatewayv1alpha2.ListenerConditionDetached))
				assert.NotNil(t, d, "detached found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionDetached), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonUnsupportedProtocol),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gatewayv1alpha2.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gatewayv1alpha2.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonReady),
					d.Reason, "reason")

				// listeners[2]: ok
				assert.Len(t, gw.Status.Listeners, 3, "conditions num")
				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gatewayv1alpha2.ListenerConditionDetached))
				assert.NotNil(t, d, "detached found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionDetached), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonAttached), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gatewayv1alpha2.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gatewayv1alpha2.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonReady),
					d.Reason, "reason")

				gw.Generation = 1
				setGatewayStatusScheduled(gw, r.op.GetControllerName())
				ready = false
				for j := range gw.Spec.Listeners {
					l := gw.Spec.Listeners[j]
					_, err := r.renderListener(gw, gwConf, &l,
						[]*gatewayv1alpha2.UDPRoute{}, addr)

					if err != nil {
						setListenerStatus(gw, &l, false, ready, 0)
						continue
					}

					setListenerStatus(gw, &l, true, ready, 2)
				}

				// test only the ready status
				assert.Len(t, gw.Status.Listeners, 3, "conditions num")
				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gatewayv1alpha2.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonPending),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gatewayv1alpha2.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonPending),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gatewayv1alpha2.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gatewayv1alpha2.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gatewayv1alpha2.ListenerReasonPending),
					d.Reason, "reason")
			},
		},
	})
}
