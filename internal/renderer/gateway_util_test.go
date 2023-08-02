package renderer

import (
	// "context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

func TestRenderGatewayUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "wrong gatewayclassname errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.GatewayClassName = "dummy"
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 0, "gw found")
			},
		},
		{
			name: "multiple gateways ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.ObjectMeta.SetName("dummy")
				gw.ObjectMeta.SetGeneration(4)
				c.gws = []gwapiv1a2.Gateway{*gw, testutils.TestGw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 2, "gw found")

				keys := []string{store.GetObjectKey(gws[0]), store.GetObjectKey(gws[1])}
				assert.Contains(t, keys,
					fmt.Sprintf("%s/%s", testutils.TestNsName, "dummy"),
					"gw 1 name found")
				assert.Contains(t, keys,
					fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					"gw 2 name found")
			},
		},
		{
			name: "gateway status ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				initGatewayStatus(gw, "dummy")
				setGatewayStatusProgrammed(gw, errors.New("dummy"))
				assert.Len(t, gw.Status.Conditions, 2, "conditions num")

				assert.Equal(t, string(gwapiv1b1.GatewayConditionAccepted),
					gw.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, int64(0), gw.Status.Conditions[0].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status,
					"status")
				assert.Equal(t, string(gwapiv1b1.GatewayReasonAccepted),
					gw.Status.Conditions[0].Reason, "reason")

				assert.Equal(t, string(gwapiv1b1.GatewayConditionProgrammed),
					gw.Status.Conditions[1].Type, "programmed")
				assert.Equal(t, int64(0), gw.Status.Conditions[1].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionFalse, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1b1.GatewayReasonInvalid),
					gw.Status.Conditions[1].Reason, "reason")
			},
		},
		{
			name: "gateway rescheduled/re-ready status ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				initGatewayStatus(gw, "dummy")
				setGatewayStatusProgrammed(gw, errors.New("dummy"))
				gw.ObjectMeta.SetGeneration(1)
				c.gws = []gwapiv1a2.Gateway{*gw}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: logr.Discard()}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				initGatewayStatus(gw, "dummy")
				setGatewayStatusProgrammed(gw, errors.New("dummy"))
				assert.Len(t, gw.Status.Conditions, 2, "conditions num")

				assert.Equal(t, string(gwapiv1b1.GatewayConditionAccepted),
					gw.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, int64(1), gw.Status.Conditions[0].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status,
					"status")
				assert.Equal(t, string(gwapiv1b1.GatewayReasonAccepted),
					gw.Status.Conditions[0].Reason, "reason")

				assert.Equal(t, string(gwapiv1b1.GatewayConditionProgrammed),
					gw.Status.Conditions[1].Type, "programmed")
				assert.Equal(t, int64(1), gw.Status.Conditions[1].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionFalse, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1b1.GatewayReasonInvalid),
					gw.Status.Conditions[1].Reason, "reason")
			},
		},
		{
			name: "listener status ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// u := testutils.TestUDPRoute.DeepCopy()
				// u.ObjectMeta.SetName("tcp")
				// u.Spec.
				// 	c.rs = []gwapiv1a2.UDPRoute{*u, testutils.TestUDPRoute}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, store.GetObjectKey(gw), fmt.Sprintf("%s/%s",
					testutils.TestNsName, "gateway-1"), "gw name found")

				initGatewayStatus(gw, config.ControllerName)
				ready := true
				for j := range gw.Spec.Listeners {
					l := gw.Spec.Listeners[j]
					addr := &addrPort{
						addr: "1.2.3.4",
						port: 1234,
					}

					_, err := r.renderListener(gw, c.gwConf, &l,
						[]*gwapiv1a2.UDPRoute{}, addr)

					if err != nil {
						setListenerStatus(gw, &l, false, ready, 0)
						continue
					}

					setListenerStatus(gw, &l, true, ready, 2)
				}

				// listeners[0]: ok
				assert.Len(t, gw.Status.Listeners, 3, "conditions num")
				d := meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1b1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonAccepted), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1b1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1b1.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonReady),
					d.Reason, "reason")

				// listeners[1]: detached
				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1b1.ListenerConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonUnsupportedProtocol),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1b1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1b1.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonReady),
					d.Reason, "reason")

				// listeners[2]: ok
				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1b1.ListenerConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonAccepted),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1b1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1b1.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonReady),
					d.Reason, "reason")

				gw.Generation = 1
				initGatewayStatus(gw, config.ControllerName)
				ready = false
				for j := range gw.Spec.Listeners {
					l := gw.Spec.Listeners[j]
					addr := &addrPort{
						addr: "1.2.3.4",
						port: 1234,
					}

					_, err := r.renderListener(gw, c.gwConf, &l,
						[]*gwapiv1a2.UDPRoute{}, addr)

					if err != nil {
						setListenerStatus(gw, &l, false, ready, 0)
						continue
					}

					setListenerStatus(gw, &l, true, ready, 2)
				}

				// test only the ready status
				assert.Len(t, gw.Status.Listeners, 3, "conditions num")
				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1b1.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonPending),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1b1.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonPending),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1b1.ListenerConditionReady))
				assert.NotNil(t, d, "ready found")
				assert.Equal(t, string(gwapiv1b1.ListenerConditionReady), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1b1.ListenerReasonPending),
					d.Reason, "reason")
			},
		},
	})
}
