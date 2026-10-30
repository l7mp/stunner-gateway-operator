package renderer

import (
	// "context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/config"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/event"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

func TestRenderGatewayUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "wrong gatewayclassname errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.GatewayClassName = "dummy"
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 0, "gw found")
			},
		},
		{
			name: "multiple gateways ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.ObjectMeta.SetName("dummy")
				gw.ObjectMeta.SetGeneration(4)
				c.gws = []gwapiv1.Gateway{*gw, testutils.TestGw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: log}

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
	})
}

// TestListenerSupportedKinds pins that a listener supports the optional TURNRoute only when the
// cluster serves its CRD.
func TestListenerSupportedKinds(t *testing.T) {
	defer func(served bool) { config.TURNRouteServed = served }(config.TURNRouteServed)
	for _, served := range []bool{false, true} {
		renderTester(t, []renderTestConfig{{
			name: fmt.Sprintf("TURNRoute CRD served: %t", served),
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				config.TURNRouteServed = served
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))
				assert.NoError(t, r.renderForGateways(c), "render success")

				gw := c.update.UpsertQueue.Gateways.Get(store.GetNamespacedName(&testutils.TestGw)).(*gwapiv1.Gateway)
				assert.NotEmpty(t, gw.Status.Listeners, "listener statuses")
				for _, ls := range gw.Status.Listeners {
					kinds := []string{}
					for _, k := range ls.SupportedKinds {
						kinds = append(kinds, string(k.Kind))
					}
					assert.Contains(t, kinds, "UDPRoute", string(ls.Name))
					if served {
						assert.Contains(t, kinds, "TURNRoute", string(ls.Name))
					} else {
						assert.NotContains(t, kinds, "TURNRoute", string(ls.Name))
					}
				}
			},
		}})
	}
}

// TestRenderGatewayStatus pins the rendered Gateway and listener statuses of a Gateway that was
// rendered before: the conditions follow the current generation.
func TestRenderGatewayStatus(t *testing.T) {
	gwKey := store.GetNamespacedName(&testutils.TestGw)
	renderTester(t, []renderTestConfig{{
		name: "re-rendered Gateway with an invalid listener",
		cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
		cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
		gws:  []gwapiv1.Gateway{testutils.TestGw},
		svcs: []corev1.Service{testutils.TestSvc},
		dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
		prep: func(c *renderTestConfig) {
			gw := testutils.TestGw.DeepCopy()
			gw.Spec.Listeners = []gwapiv1.Listener{
				{Name: "gateway-1-listener-udp", Port: 1, Protocol: "TURN-UDP"},
				{Name: "invalid", Port: 3, Protocol: "dummy"},
				{Name: "gateway-1-listener-tcp", Port: 2, Protocol: "TURN-TCP"},
			}
			// The status of an earlier render, at an earlier generation.
			initGatewayStatus(gw, nil)
			setGatewayStatusProgrammed(gw, errors.New("dummy"), nil)
			gw.SetGeneration(1)
			c.gws = []gwapiv1.Gateway{*gw}
		},
		tester: func(t *testing.T, r *renderer) {
			gc, err := r.getGatewayClass()
			assert.NoError(t, err, "gw-class found")
			c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
			c.gwConf, err = r.getGatewayConfig4Class(c)
			assert.NoError(t, err, "gw-conf found")
			c.update = event.NewEventUpdate(0)
			c.gws.ResetGateways(r.getGateways4Class(c))
			assert.NoError(t, r.renderForGateways(c), "render success")

			gw := c.update.UpsertQueue.Gateways.Get(gwKey).(*gwapiv1.Gateway)
			d := meta.FindStatusCondition(gw.Status.Conditions, string(gwapiv1.GatewayConditionAccepted))
			assert.NotNil(t, d, "accepted found")
			assert.Equal(t, metav1.ConditionTrue, d.Status, "accepted")
			assert.Equal(t, string(gwapiv1.GatewayReasonAccepted), d.Reason, "reason")
			assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
			d = meta.FindStatusCondition(gw.Status.Conditions, string(gwapiv1.GatewayConditionProgrammed))
			assert.NotNil(t, d, "programmed found")
			assert.Equal(t, int64(1), d.ObservedGeneration, "gen")

			assert.Len(t, gw.Status.Listeners, 3, "listener statuses")
			for _, i := range []int{0, 2} {
				ls := gw.Status.Listeners[i]
				for cond, want := range map[gwapiv1.ListenerConditionType][2]string{
					gwapiv1.ListenerConditionAccepted:     {"True", string(gwapiv1.ListenerReasonAccepted)},
					gwapiv1.ListenerConditionResolvedRefs: {"True", string(gwapiv1.ListenerReasonResolvedRefs)},
					gwapiv1.ListenerConditionConflicted:   {"False", string(gwapiv1.ListenerReasonNoConflicts)},
				} {
					d := meta.FindStatusCondition(ls.Conditions, string(cond))
					if assert.NotNil(t, d, string(ls.Name)+"/"+string(cond)) {
						assert.Equal(t, want[0], string(d.Status), string(ls.Name)+"/"+string(cond))
						assert.Equal(t, want[1], d.Reason, string(ls.Name)+"/"+string(cond))
						assert.Equal(t, int64(1), d.ObservedGeneration, string(ls.Name)+"/"+string(cond))
					}
				}
			}
			ls := gw.Status.Listeners[1]
			d = meta.FindStatusCondition(ls.Conditions, string(gwapiv1.ListenerConditionAccepted))
			assert.NotNil(t, d, "accepted found")
			assert.Equal(t, metav1.ConditionFalse, d.Status, "unknown protocol not accepted")
			assert.Equal(t, string(gwapiv1.ListenerReasonUnsupportedProtocol), d.Reason, "reason")
			d = meta.FindStatusCondition(ls.Conditions, string(gwapiv1.ListenerConditionResolvedRefs))
			assert.NotNil(t, d, "resolved-refs found")
			assert.Equal(t, metav1.ConditionTrue, d.Status, "resolved")
		},
	}})
}

// TestRenderGatewayStatusRollout pins the Programmed condition against the rollout of the dataplane
// Deployment: a stuck rollout is not programmed, a managed-dataplane-disabled Gateway ignores it.
func TestRenderGatewayStatusRollout(t *testing.T) {
	defer store.Deployments.Flush()
	gwKey := store.GetNamespacedName(&testutils.TestGw)
	at := gwapiv1.HostnameAddressType
	for _, tc := range []struct {
		name       string
		conditions []appv1.DeploymentCondition
		disabled   bool
		status     metav1.ConditionStatus
		reason     string
		message    string
	}{
		{name: "no Deployment yet", status: metav1.ConditionTrue},
		{name: "a rollout in progress", status: metav1.ConditionTrue,
			conditions: []appv1.DeploymentCondition{{Type: appv1.DeploymentProgressing,
				Status: corev1.ConditionTrue, Reason: "ReplicaSetUpdated"}}},
		{name: "a stalled rollout", status: metav1.ConditionFalse, reason: string(gwapiv1.GatewayReasonPending),
			conditions: []appv1.DeploymentCondition{{Type: appv1.DeploymentProgressing,
				Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"}}},
		{name: "pods cannot be created", status: metav1.ConditionFalse, message: "exceeded quota",
			conditions: []appv1.DeploymentCondition{{Type: appv1.DeploymentReplicaFailure,
				Status: corev1.ConditionTrue, Message: "exceeded quota"}}},
		{name: "no managed dataplane", status: metav1.ConditionTrue, disabled: true,
			conditions: []appv1.DeploymentCondition{{Type: appv1.DeploymentReplicaFailure,
				Status: corev1.ConditionTrue, Message: "exceeded quota"}}},
	} {
		renderTester(t, []renderTestConfig{{
			name: tc.name,
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Addresses = []gwapiv1.GatewaySpecAddress{{Type: &at, Value: "turn.example.com"}}
				// The Service owned by the Gateway exposes it.
				s := testutils.TestSvc.DeepCopy()
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}
				if tc.disabled {
					gw.SetAnnotations(map[string]string{
						opdefault.ManagedDataplaneDisabledAnnotationKey: opdefault.ManagedDataplaneDisabledAnnotationValue,
					})
				}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				store.Deployments.Flush()
				if tc.conditions != nil {
					one := int32(1)
					dep := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: gwKey.Namespace, Name: gwKey.Name},
						Spec: appv1.DeploymentSpec{Replicas: &one}}
					dep.Status.Conditions = tc.conditions
					store.Deployments.Reset([]client.Object{dep})
				}

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))
				assert.NoError(t, r.renderForGateways(c), "render success")

				gw := c.update.UpsertQueue.Gateways.Get(gwKey).(*gwapiv1.Gateway)
				d := meta.FindStatusCondition(gw.Status.Conditions, string(gwapiv1.GatewayConditionProgrammed))
				if !assert.NotNil(t, d, "programmed found") {
					return
				}
				assert.Equal(t, tc.status, d.Status, "programmed")
				if tc.reason != "" {
					assert.Equal(t, tc.reason, d.Reason, "reason")
				}
				assert.Contains(t, d.Message, tc.message, "message")
			},
		}})
	}
}
