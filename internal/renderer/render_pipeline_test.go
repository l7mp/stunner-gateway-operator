package renderer

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/config"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/event"
	licensemgr "github.com/l7mp/stunner-gateway-operator/v2/internal/licensemanager"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

// TestRenderPipeline drives the whole render loop: a render event in, an update event out.
func TestRenderPipeline(t *testing.T) {
	gwKey := store.GetNamespacedName(&testutils.TestGw)
	clsKey := store.GetNamespacedName(&testutils.TestGwClass)

	renderTester(t, []renderTestConfig{
		{
			name: "full render",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)

				// An event of an unknown type is ignored.
				r.GetRenderChannel() <- event.NewEventUpdate(0)
				r.GetRenderChannel() <- event.NewEventRender(7)

				var e event.Event
				select {
				case e = <-opCh:
				case <-time.After(5 * time.Second):
					assert.FailNow(t, "no update from the renderer")
				}
				u, ok := e.(*event.EventUpdate)
				assert.True(t, ok, "update event")
				assert.True(t, u.RequestAck, "update requests an ack")
				assert.Equal(t, 7, u.Generation, "generation")

				assert.Len(t, u.ConfigQueue, 1, "one dataplane config")
				assert.Equal(t, store.GetObjectKey(&testutils.TestGw), u.ConfigQueue[0].Admin.Name, "config name")
				assert.NotNil(t, u.UpsertQueue.Deployments.Get(gwKey), "deployment upserted")
				assert.NotNil(t, u.UpsertQueue.Services.Get(gwKey), "service upserted")

				gc, ok := u.UpsertQueue.GatewayClasses.Get(clsKey).(*gwapiv1.GatewayClass)
				assert.True(t, ok, "gateway class upserted")
				cond := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				assert.NotNil(t, cond, "accepted condition")
				assert.Equal(t, "True", string(cond.Status), "class accepted")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonAccepted), cond.Reason, "reason")
				assert.Equal(t, int64(0), cond.ObservedGeneration, "gen")
			},
		},
		{
			name: "re-rendered class follows its generation",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				// The status of an earlier render, at an earlier generation.
				cls := testutils.TestGwClass.DeepCopy()
				setGatewayClassStatusAccepted(cls, nil)
				cls.SetGeneration(1)
				c.cls = []gwapiv1.GatewayClass{*cls}
			},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)
				r.GetRenderChannel() <- event.NewEventRender(1)

				var e event.Event
				select {
				case e = <-opCh:
				case <-time.After(5 * time.Second):
					assert.FailNow(t, "no update from the renderer")
				}
				u, ok := e.(*event.EventUpdate)
				assert.True(t, ok, "update event")

				gc, ok := u.UpsertQueue.GatewayClasses.Get(clsKey).(*gwapiv1.GatewayClass)
				assert.True(t, ok, "gateway class upserted")
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				cond := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				assert.NotNil(t, cond, "accepted condition")
				assert.Equal(t, "True", string(cond.Status), "class accepted")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonAccepted), cond.Reason, "reason")
				assert.Equal(t, int64(1), cond.ObservedGeneration, "gen")
			},
		},
		{
			name: "missing default Dataplane invalidates the class",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)
				r.GetRenderChannel() <- event.NewEventRender(1)

				var e event.Event
				select {
				case e = <-opCh:
				case <-time.After(5 * time.Second):
					assert.FailNow(t, "no update from the renderer")
				}
				u := e.(*event.EventUpdate)

				assert.Len(t, u.ConfigQueue, 0, "no dataplane config")
				assert.Equal(t, 0, u.UpsertQueue.Deployments.Len(), "no deployment upserted")
				assert.NotNil(t, u.DeleteQueue.Deployments.Get(gwKey), "deployment deleted")
				assert.NotNil(t, u.DeleteQueue.DaemonSets.Get(gwKey), "daemonset deleted")
				assert.NotNil(t, u.DeleteQueue.Services.Get(gwKey), "service deleted")

				gc := u.UpsertQueue.GatewayClasses.Get(clsKey).(*gwapiv1.GatewayClass)
				cond := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				assert.NotNil(t, cond, "accepted condition")
				assert.Equal(t, "False", string(cond.Status), "class not accepted")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonInvalidParameters), cond.Reason, "reason")
				assert.True(t, strings.Contains(cond.Message, "missing Dataplane"), "message")

				gw := u.UpsertQueue.Gateways.Get(gwKey).(*gwapiv1.Gateway)
				cond = meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				assert.NotNil(t, cond, "programmed condition")
				assert.Equal(t, "False", string(cond.Status), "gateway not programmed")
			},
		},
		{
			name: "missing named Dataplane invalidates the class",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				dp := "missing-dataplane"
				w.Spec.Dataplane = &dp
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)
				r.GetRenderChannel() <- event.NewEventRender(1)

				var e event.Event
				select {
				case e = <-opCh:
				case <-time.After(5 * time.Second):
					assert.FailNow(t, "no update from the renderer")
				}
				u := e.(*event.EventUpdate)

				assert.Len(t, u.ConfigQueue, 0, "no dataplane config")
				assert.NotNil(t, u.DeleteQueue.Deployments.Get(gwKey), "deployment deleted")

				gc := u.UpsertQueue.GatewayClasses.Get(clsKey).(*gwapiv1.GatewayClass)
				cond := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				assert.Equal(t, "False", string(cond.Status), "class not accepted")
			},
		},
		{
			name: "missing GatewayConfig invalidates the class",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)
				r.GetRenderChannel() <- event.NewEventRender(1)

				var e event.Event
				select {
				case e = <-opCh:
				case <-time.After(5 * time.Second):
					assert.FailNow(t, "no update from the renderer")
				}
				u := e.(*event.EventUpdate)

				assert.Len(t, u.ConfigQueue, 0, "no dataplane config")
				gc := u.UpsertQueue.GatewayClasses.Get(clsKey).(*gwapiv1.GatewayClass)
				cond := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				assert.Equal(t, "False", string(cond.Status), "class not accepted")
				assert.NotNil(t, u.DeleteQueue.Services.Get(gwKey), "service deleted")
			},
		},
		{
			name: "no GatewayClass renders nothing",
			cls:  []gwapiv1.GatewayClass{},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)
				r.GetRenderChannel() <- event.NewEventRender(1)

				select {
				case <-opCh:
					assert.Fail(t, "unexpected update without a GatewayClass")
				case <-time.After(200 * time.Millisecond):
				}

				assert.Nil(t, r.Finalize(2), "nothing to finalize")
			},
		},
		{
			name: "finalize invalidates every class",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				u := r.Finalize(3)
				assert.NotNil(t, u, "finalize update")
				assert.Equal(t, 3, u.Generation, "generation")

				gc := u.UpsertQueue.GatewayClasses.Get(clsKey).(*gwapiv1.GatewayClass)
				cond := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				assert.Equal(t, "False", string(cond.Status), "class not accepted")
				assert.True(t, strings.Contains(cond.Message, "Operator unavailable"), "message")

				assert.NotNil(t, u.UpsertQueue.Gateways.Get(gwKey), "gateway status updated")
				assert.NotNil(t, u.DeleteQueue.Deployments.Get(gwKey), "deployment deleted")
				assert.NotNil(t, u.DeleteQueue.DaemonSets.Get(gwKey), "daemonset deleted")
				assert.NotNil(t, u.DeleteQueue.Services.Get(gwKey), "service deleted")
			},
		},
		{
			name: "a Gateway with a parametersRef is invalidated",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Infrastructure = &gwapiv1.GatewayInfrastructure{
					ParametersRef: &gwapiv1.LocalParametersReference{
						Group: "stunner.l7mp.io",
						Kind:  "SIPTrunk",
						Name:  "trunk",
					},
				}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)
				r.GetRenderChannel() <- event.NewEventRender(1)

				var e event.Event
				select {
				case e = <-opCh:
				case <-time.After(5 * time.Second):
					assert.FailNow(t, "no update from the renderer")
				}
				u := e.(*event.EventUpdate)

				assert.Len(t, u.ConfigQueue, 0, "no dataplane config")
				assert.NotNil(t, u.DeleteQueue.Deployments.Get(gwKey), "deployment deleted")
				gw := u.UpsertQueue.Gateways.Get(gwKey).(*gwapiv1.Gateway)
				cond := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionAccepted))
				assert.NotNil(t, cond, "accepted condition")
				assert.Equal(t, "False", string(cond.Status), "gateway not accepted")
				assert.Equal(t, string(gwapiv1.GatewayReasonInvalidParameters), cond.Reason, "reason")
				assert.Contains(t, cond.Message, "SIPTrunk", "message")

				// The class itself stays accepted.
				gc := u.UpsertQueue.GatewayClasses.Get(clsKey).(*gwapiv1.GatewayClass)
				cond = meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				assert.Equal(t, "True", string(cond.Status), "class accepted")
			},
		},
		{
			name: "an invalid authentication invalidates the Gateway",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				atype := "dummy"
				w.Spec.AuthType = &atype
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				r.licmgr = licensemgr.NewStubManager("", log)
				opCh := make(chan event.Event, 10)
				r.SetOperatorChannel(opCh)
				r.GetRenderChannel() <- event.NewEventRender(1)

				var e event.Event
				select {
				case e = <-opCh:
				case <-time.After(5 * time.Second):
					assert.FailNow(t, "no update from the renderer")
				}
				u := e.(*event.EventUpdate)

				assert.Len(t, u.ConfigQueue, 0, "no dataplane config")
				assert.NotNil(t, u.DeleteQueue.Deployments.Get(gwKey), "deployment deleted")
				gw := u.UpsertQueue.Gateways.Get(gwKey).(*gwapiv1.Gateway)
				cond := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				assert.NotNil(t, cond, "programmed condition")
				assert.Equal(t, "False", string(cond.Status), "gateway not programmed")
				assert.True(t, strings.Contains(cond.Message, "invalid authentication type"), "message")
			},
		},
	})
}

// TestRenderPipelineFeatures renders whole Gateways and checks the documented features in the
// rendered config, the dataplane and the statuses.
func TestRenderPipelineFeatures(t *testing.T) {
	gwKey := store.GetNamespacedName(&testutils.TestGw)

	renderTester(t, []renderTestConfig{
		{
			name: "a Hostname address is advertised but never requested as loadBalancerIP",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				at := gwapiv1.HostnameAddressType
				gw.Spec.Addresses = []gwapiv1.GatewaySpecAddress{{Type: &at, Value: "turn.example.com"}}
				c.gws = []gwapiv1.Gateway{*gw}

				// The Service owned by the Gateway exposes it.
				s := testutils.TestSvc.DeepCopy()
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}
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
				conf := c.update.ConfigQueue[0]
				for _, lc := range conf.Listeners {
					assert.Equal(t, "turn.example.com", lc.PublicAddr, "public address")
				}

				svc := c.update.UpsertQueue.Services.Get(gwKey).(*corev1.Service)
				assert.Empty(t, svc.Spec.LoadBalancerIP, "no loadBalancerIP")

				gw := c.update.UpsertQueue.Gateways.Get(gwKey).(*gwapiv1.Gateway)
				assert.Len(t, gw.Status.Addresses, 1, "status address")
				assert.Equal(t, gwapiv1.HostnameAddressType, *gw.Status.Addresses[0].Type, "address type")
				assert.Equal(t, "turn.example.com", gw.Status.Addresses[0].Value, "address value")
			},
		},
		{
			name: "a backend of a wrong kind sets ResolvedRefs=False with reason InvalidKind",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				kind := gwapiv1.Kind("Pod")
				udp.Spec.Rules[0].BackendRefs[0].Kind = &kind
				c.rs = []stnrgwv1.UDPRoute{*udp}
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
				ro := c.update.UpsertQueue.UDPRoutes.Get(store.GetNamespacedName(&testutils.TestUDPRoute)).(*stnrgwv1.UDPRoute)
				assert.Len(t, ro.Status.Parents, 1, "parent status")
				cond := meta.FindStatusCondition(ro.Status.Parents[0].Conditions,
					string(gwapiv1.RouteConditionResolvedRefs))
				assert.NotNil(t, cond, "resolved-refs condition")
				assert.Equal(t, "False", string(cond.Status), "refs not resolved")
				assert.Equal(t, string(gwapiv1.RouteReasonInvalidKind), cond.Reason, "reason")
			},
		},
		{
			name: "tolerations and the default termination grace period",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				dp := testutils.TestDataplane.DeepCopy()
				dp.Spec.TerminationGracePeriodSeconds = nil
				dp.Spec.Tolerations = []corev1.Toleration{{
					Key:      "dedicated",
					Operator: corev1.TolerationOpEqual,
					Value:    "media",
					Effect:   corev1.TaintEffectNoSchedule,
				}}
				c.dps = []stnrgwv1.Dataplane{*dp}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))

				obj, err := r.generateDataplane(c)
				assert.NoError(t, err, "create deployment")
				podSpec := obj.(*appv1.Deployment).Spec.Template.Spec
				assert.Equal(t, []corev1.Toleration{{
					Key:      "dedicated",
					Operator: corev1.TolerationOpEqual,
					Value:    "media",
					Effect:   corev1.TaintEffectNoSchedule,
				}}, podSpec.Tolerations, "tolerations")
				assert.NotNil(t, podSpec.TerminationGracePeriodSeconds, "grace set")
				assert.Equal(t, config.TerminationGrace, *podSpec.TerminationGracePeriodSeconds, "default grace")
				assert.Equal(t, int64(3600), *podSpec.TerminationGracePeriodSeconds, "3600 seconds")
			},
		},
		{
			name:  "plain TCP, TLS and DTLS listeners render L4 servers",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			trs:   []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			esls:  []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			scrts: []corev1.Secret{testutils.TestSecret},
			dps:   []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tcp"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TCP"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("TLS"),
					TLS:      &tls,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:     gwapiv1.PortNumber(4),
					Protocol: gwapiv1.ProtocolType("DTLS"),
					TLS:      &tls,
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				tr := testutils.TestTCPRoute.DeepCopy()
				port := gwapiv1.PortNumber(2) // the Service port "tcp-ok"
				tr.Spec.Rules[0].BackendRefs[0].Port = &port
				c.trs = []stnrgwv1.TCPRoute{*tr}
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
				conf := c.update.ConfigQueue[0]
				assert.Len(t, conf.Listeners, 3, "listener num")

				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "tcp name")
				assert.Equal(t, "TCP", lc.Protocol, "tcp proto")
				assert.Nil(t, lc.TLS, "no tls on tcp")
				assert.Equal(t, stnrapiv2.ServerTypeL4.String(), serverOf(conf, lc).Type, "tcp server type")
				assert.Equal(t, []string{"testnamespace/tcproute-ok/tcp"}, serverOf(conf, lc).Clusters, "tcp cluster")

				lc = conf.Listeners[1]
				assert.Equal(t, "TLS", lc.Protocol, "tls proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(&lc).Cert, "tls cert")
				assert.Equal(t, testutils.TestKey64, tlsOf(&lc).Key, "tls key")
				assert.Equal(t, stnrapiv2.ServerTypeL4.String(), serverOf(conf, lc).Type, "tls server type")

				lc = conf.Listeners[2]
				assert.Equal(t, "DTLS", lc.Protocol, "dtls proto")
				assert.Equal(t, testutils.TestCert64, tlsOf(&lc).Cert, "dtls cert")
				assert.Equal(t, stnrapiv2.ServerTypeL4.String(), serverOf(conf, lc).Type, "dtls server type")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/tcproute-ok/tcp", rc.Name, "cluster name")
				assert.ElementsMatch(t, []string{"1.2.3.4:2", "1.2.3.5:2"}, rc.Endpoints, "endpoints")
			},
		},
		{
			name:     "a Gateway API TCPRoute masked by a STUNner TCPRoute",
			cls:      []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:      []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:      []gwapiv1.Gateway{testutils.TestGw},
			trs:      []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			trsGwAPI: []stnrgwv1.TCPRoute{testutils.TestTCPRoute},
			svcs:     []corev1.Service{testutils.TestSvc},
			esls:     []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:      []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-tcp"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TCP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
				port := gwapiv1.PortNumber(2)
				tr := testutils.TestTCPRoute.DeepCopy()
				tr.Spec.Rules[0].BackendRefs[0].Port = &port
				c.trs = []stnrgwv1.TCPRoute{*tr}
				c.trsGwAPI = []stnrgwv1.TCPRoute{*tr.DeepCopy()}
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
				conf := c.update.ConfigQueue[0]
				assert.Len(t, conf.Clusters, 1, "one cluster: the masked route is skipped")

				key := store.GetNamespacedName(&testutils.TestTCPRoute)
				assert.NotNil(t, c.update.UpsertQueue.TCPRoutes.Get(key), "stunner route status")
				ro, ok := c.update.UpsertQueue.TCPRoutesGwAPI.Get(key).(*gwapiv1.TCPRoute)
				assert.True(t, ok, "masked route status")
				assert.Len(t, ro.Status.Parents, 1, "masked route parent status")
			},
		},
		{
			name: "routes of foreign and missing Gateways are ignored",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				other := testutils.TestGwClass.DeepCopy()
				other.SetName("other-class")
				other.Spec.ControllerName = gwapiv1.GatewayController("example.com/other")
				c.cls = []gwapiv1.GatewayClass{testutils.TestGwClass, *other}

				gw2 := testutils.TestGw.DeepCopy()
				gw2.SetName("gateway-2")
				gw2.Spec.GatewayClassName = "other-class"
				c.gws = []gwapiv1.Gateway{testutils.TestGw, *gw2}

				foreign := testutils.TestUDPRoute.DeepCopy()
				foreign.SetName("udproute-foreign")
				foreign.Spec.ParentRefs[0].Name = "gateway-2"
				orphan := testutils.TestUDPRoute.DeepCopy()
				orphan.SetName("udproute-orphan")
				orphan.Spec.ParentRefs[0].Name = "gateway-missing"
				c.rs = []stnrgwv1.UDPRoute{testutils.TestUDPRoute, *foreign, *orphan}
			},
			tester: func(t *testing.T, r *renderer) {
				gcs := r.getGatewayClasses()
				assert.Len(t, gcs, 1, "only our class")
				gc := gcs[0]
				var err error
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "only our gateway")
				c.gws.ResetGateways(gws)

				assert.NoError(t, r.renderForGateways(c), "render success")
				conf := c.update.ConfigQueue[0]
				assert.Len(t, conf.Clusters, 1, "cluster num")
				assert.Equal(t, "testnamespace/udproute-ok/udp", conf.Clusters[0].Name, "our route")
				assert.Equal(t, 1, c.update.UpsertQueue.UDPRoutes.Len(), "no status for foreign routes")
			},
		},
		{
			name: "listener errors go to the listener status",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Name: gwapiv1.ObjectName("missing-secret"),
					}},
				}
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("conflicting"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("unknown-protocol"),
					Port:     gwapiv1.PortNumber(5),
					Protocol: gwapiv1.ProtocolType("HTTP"),
				}, {
					Name:     gwapiv1.SectionName("tls-no-secret"),
					Port:     gwapiv1.PortNumber(6),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					TLS:      &tls,
				}}
				// Stale conditions of the Gateway are pruned to the limit.
				for i := 0; i < 10; i++ {
					gw.Status.Conditions = append(gw.Status.Conditions, metav1.Condition{
						Type:   fmt.Sprintf("Stale%d", i),
						Status: metav1.ConditionTrue,
						Reason: "Stale",
					})
				}
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
				conf := c.update.ConfigQueue[0]
				assert.Len(t, conf.Listeners, 1, "only the valid listener")
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", conf.Listeners[0].Name, "listener")

				gw := c.update.UpsertQueue.Gateways.Get(gwKey).(*gwapiv1.Gateway)
				assert.LessOrEqual(t, len(gw.Status.Conditions), 8, "conditions pruned")
				assert.Len(t, gw.Status.Listeners, 4, "listener statuses")

				cond := meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.Equal(t, "True", string(cond.Status), "conflicted")
				cond = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.Equal(t, string(gwapiv1.ListenerReasonPortUnavailable), cond.Reason, "port unavailable")

				cond = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.Equal(t, "False", string(cond.Status), "unknown protocol not accepted")
				assert.Equal(t, string(gwapiv1.ListenerReasonUnsupportedProtocol), cond.Reason, "unsupported")

				cond = meta.FindStatusCondition(gw.Status.Listeners[3].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.Equal(t, "False", string(cond.Status), "certificate not resolved")
				assert.Equal(t, string(gwapiv1.ListenerReasonInvalidCertificateRef), cond.Reason, "invalid cert ref")
			},
		},
	})
}

// TestRenderEdgeCases covers the less common inputs of the cluster, public address, Service and
// dataplane renderers.
func TestRenderEdgeCases(t *testing.T) {
	ownedSvc := func(name string, stype corev1.ServiceType) corev1.Service {
		s := testutils.TestSvc.DeepCopy()
		s.SetName(name)
		s.Spec.Type = stype
		s.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion: gwapiv1.GroupVersion.String(),
			Kind:       "Gateway",
			UID:        testutils.TestGw.GetUID(),
			Name:       testutils.TestGw.GetName(),
		}})
		return *s
	}

	renderTester(t, []renderTestConfig{
		{
			name: "a route without rules errs, a route with more rules uses the first",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				empty := testutils.TestUDPRoute.DeepCopy()
				empty.SetName("udproute-empty")
				empty.Spec.Rules = []stnrgwv1.RouteRule{}
				multi := testutils.TestUDPRoute.DeepCopy()
				multi.SetName("udproute-multi")
				multi.Spec.Rules = append(multi.Spec.Rules, stnrgwv1.RouteRule{
					BackendRefs: []stnrgwv1.BackendRef{{
						BackendObjectReference: stnrgwv1.BackendObjectReference{Name: "ignored"},
					}},
				})
				c.rs = []stnrgwv1.UDPRoute{*empty, *multi}
			},
			tester: func(t *testing.T, r *renderer) {
				for _, ro := range r.allRoutes() {
					rc, _, err := r.renderCluster(nil, ro)
					switch ro.GetName() {
					case "udproute-empty":
						assert.Nil(t, rc, "no cluster")
						assert.True(t, IsCritical(err), "critical err")
						assert.Equal(t, "no rules found in route", err.Error(), "message")
					case "udproute-multi":
						assert.False(t, IsCritical(err), "no critical error")
						assert.ElementsMatch(t, []string{"1.2.3.4", "1.2.3.5"}, rc.Endpoints, "first rule only")
					}
				}
			},
		},
		{
			name: "a Service kind in the STUNner group is an invalid backend kind",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("Service")
				udp.Spec.Rules[0].BackendRefs[0].Group = &group
				udp.Spec.Rules[0].BackendRefs[0].Kind = &kind
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 1, "route len")
				rc, _, err := r.renderCluster(nil, rs[0])
				assert.True(t, IsNonCriticalError(err, InvalidBackendKind), "invalid kind")
				assert.True(t, strings.Contains(err.Error(), "invalid Kind"), "message")
				assert.Empty(t, rc.Endpoints, "no endpoints")
			},
		},
		{
			name:   "TURNRoute tunnel errors",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			turnrs: []stnrgwv1.TURNRoute{testutils.TestTURNRoute},
			tscrts: []corev1.Secret{testutils.TestTunnelSecret},
			prep: func(c *renderTestConfig) {
				badURI := testutils.TestTURNRoute.DeepCopy()
				badURI.SetName("turnroute-bad-uri")
				badURI.Spec.Tunnel.URI = "http://example.com"
				noAuth := testutils.TestTURNRoute.DeepCopy()
				noAuth.SetName("turnroute-no-auth")
				noAuth.Spec.Tunnel.AuthRef = &gwapiv1.SecretObjectReference{Name: "missing-secret"}
				badAuth := testutils.TestTURNRoute.DeepCopy()
				badAuth.SetName("turnroute-bad-auth")
				badAuth.Spec.Tunnel.AuthRef = &gwapiv1.SecretObjectReference{Name: "bad-tunnel-secret"}
				noBackend := testutils.TestTURNRoute.DeepCopy()
				noBackend.SetName("turnroute-no-backend")
				noBackend.Spec.Rules = []stnrgwv1.RouteRule{{
					BackendRefs: []stnrgwv1.BackendRef{{
						BackendObjectReference: stnrgwv1.BackendObjectReference{Name: "missing-service"},
					}},
				}}
				c.turnrs = []stnrgwv1.TURNRoute{*badURI, *noAuth, *badAuth, *noBackend}

				bad := testutils.TestTunnelSecret.DeepCopy()
				bad.SetName("bad-tunnel-secret")
				bad.Data = map[string][]byte{"type": []byte("dummy")}
				c.tscrts = []corev1.Secret{testutils.TestTunnelSecret, *bad}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 4, "route len")
				for _, ro := range rs {
					rc, _, err := r.renderCluster(nil, ro)
					switch ro.GetName() {
					case "turnroute-bad-uri":
						assert.Nil(t, rc, "no cluster")
						assert.Equal(t, "invalid tunnel URI (expecting a TURN URI)", err.Error(), "bad uri")
					case "turnroute-no-auth":
						assert.Nil(t, rc, "no cluster")
						assert.Equal(t, "missing or invalid Secret for the tunnel authentication", err.Error(), "no auth")
					case "turnroute-bad-auth":
						assert.Nil(t, rc, "no cluster")
						assert.Equal(t, "invalid tunnel authentication in Secret", err.Error(), "bad auth")
					case "turnroute-no-backend":
						assert.Empty(t, rc.Endpoints, "a tunnel cluster without endpoints")
						assert.True(t, IsNonCriticalError(err, BackendNotFound), "no backend")
						assert.Equal(t, "backend not found", err.Error(), "no backend message")
					}
				}
			},
		},
		{
			name: "the public address comes from the preferred Service",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			prep: func(c *renderTestConfig) {
				lb := ownedSvc("svc-lb", corev1.ServiceTypeLoadBalancer)
				c.svcs = []corev1.Service{
					ownedSvc("svc-clusterip", corev1.ServiceTypeClusterIP),
					ownedSvc("svc-nodeport", corev1.ServiceTypeNodePort),
					lb,
					ownedSvc("svc-externalname", corev1.ServiceTypeExternalName),
				}
			},
			tester: func(t *testing.T, r *renderer) {
				aps, err := r.getPublicAddr(&testutils.TestGw)
				assert.NoError(t, err, "public address")
				assert.Len(t, aps, 2, "per listener")
				assert.Equal(t, "1.2.3.4", aps[0].addr, "the LoadBalancer address")
				assert.Equal(t, gwapiv1.IPAddressType, aps[0].aType, "address type")
			},
		},
		{
			name: "a LoadBalancer hostname with and without a matching port status",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			prep: func(c *renderTestConfig) {
				s := ownedSvc("svc-lb", corev1.ServiceTypeLoadBalancer)
				s.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{
					// The status of the second Service port names another port.
					Hostname: "lb.example.com",
					Ports: []corev1.PortStatus{
						{Port: 1, Protocol: corev1.ProtocolUDP},
						{Port: 99, Protocol: corev1.ProtocolTCP},
					},
				}}
				c.svcs = []corev1.Service{s}
			},
			tester: func(t *testing.T, r *renderer) {
				aps, err := r.getPublicAddr(&testutils.TestGw)
				assert.NoError(t, err, "public address")
				for i := range aps {
					assert.Equal(t, "lb.example.com", aps[i].addr, "hostname")
					assert.Equal(t, gwapiv1.HostnameAddressType, aps[i].aType, "hostname type")
				}
				assert.Equal(t, 2, aps[1].port, "the Service port of the mismatched status")
			},
		},
		{
			name: "no public address without a LoadBalancer status or a node",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			prep: func(c *renderTestConfig) {
				c.svcs = []corev1.Service{ownedSvc("svc-clusterip", corev1.ServiceTypeClusterIP)}
			},
			tester: func(t *testing.T, r *renderer) {
				aps, err := r.getPublicAddr(&testutils.TestGw)
				assert.True(t, IsNonCriticalError(err, PublicListenerAddressNotFound), "no address")
				for i := range aps {
					assert.True(t, aps[i].isEmpty(), "empty address")
				}
			},
		},
		{
			name: "Service annotations: an explicit type, invalid ports and an existing health check port",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-port":     "8080",
					"service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol": "HTTP",
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}
				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey:   "LoadBalancer",
					opdefault.NodePortAnnotationKey:      "not-json",
					opdefault.TargetPortAnnotationKey:    "not-json",
					opdefault.MixedProtocolAnnotationKey: "true",
				})
				c.gws = []gwapiv1.Gateway{*gw}
				// The existing Service of the Gateway already has the health check port.
				s := ownedSvc(testutils.TestGw.GetName(), corev1.ServiceTypeLoadBalancer)
				s.Spec.Ports = append(s.Spec.Ports, corev1.ServicePort{
					Name: "gateway-health-check", Protocol: corev1.ProtocolTCP, Port: 1234,
				})
				c.svcs = []corev1.Service{s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				s, tp := r.createLbService4Gateway(c, gws[0])
				assert.NotNil(t, s, "svc create")
				assert.Empty(t, tp, "invalid targetport annotation ignored")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "explicit type")
				hc := 0
				for _, p := range s.Spec.Ports {
					assert.Zero(t, p.NodePort, "invalid nodeport annotation ignored")
					if p.Name == "gateway-health-check" {
						hc++
						assert.Equal(t, int32(8080), p.Port, "health check port updated")
					}
				}
				assert.Equal(t, 1, hc, "one health check port")

				// A Gateway without listeners gets no Service.
				gw := gws[0].DeepCopy()
				gw.Spec.Listeners = nil
				s, _ = r.createLbService4Gateway(c, gw)
				assert.Nil(t, s, "no svc without listeners")
			},
		},
		{
			name: "dataplane affinity and a missing Dataplane",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				dp := testutils.TestDataplane.DeepCopy()
				dp.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "media", Operator: corev1.NodeSelectorOpExists,
							}},
						}},
					},
				}}
				c.dps = []stnrgwv1.Dataplane{*dp}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.gws.ResetGateways(r.getGateways4Class(c))

				obj, err := r.generateDataplane(c)
				assert.NoError(t, err, "create deployment")
				aff := obj.(*appv1.Deployment).Spec.Template.Spec.Affinity
				assert.NotNil(t, aff, "affinity")
				assert.Equal(t, "media", aff.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.
					NodeSelectorTerms[0].MatchExpressions[0].Key, "affinity copied")

				store.Dataplanes.Flush()
				obj, err = r.generateDataplane(c)
				assert.Nil(t, obj, "no dataplane")
				assert.True(t, IsCritical(err), "critical err")
				assert.Equal(t, "missing Dataplane resource for Gateway", err.Error(), "message")
			},
		},
		{
			name: "namespace Selector attachment policies that admit nothing",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			nss:  []corev1.Namespace{testutils.TestNs},
			prep: func(c *renderTestConfig) {
				from := gwapiv1.NamespacesFromSelector
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     "no-selector",
					Port:     1,
					Protocol: "TURN-UDP",
					AllowedRoutes: &gwapiv1.AllowedRoutes{Namespaces: &gwapiv1.RouteNamespaces{
						From: &from,
					}},
				}, {
					Name:     "invalid-selector",
					Port:     2,
					Protocol: "TURN-UDP",
					AllowedRoutes: &gwapiv1.AllowedRoutes{Namespaces: &gwapiv1.RouteNamespaces{
						From: &from,
						Selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{
							Key: "a", Operator: "Bogus",
						}}},
					}},
				}, {
					Name:     "no-match",
					Port:     3,
					Protocol: "TURN-UDP",
					AllowedRoutes: &gwapiv1.AllowedRoutes{Namespaces: &gwapiv1.RouteNamespaces{
						From: &from,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"no-such-label": "x"},
						},
					}},
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.ParentRefs = []gwapiv1.ParentReference{
					{Name: "gateway-1", SectionName: new(gwapiv1.SectionName("no-selector"))},
					{Name: "gateway-1", SectionName: new(gwapiv1.SectionName("invalid-selector"))},
					{Name: "gateway-1", SectionName: new(gwapiv1.SectionName("no-match"))},
				}
				orphan := udp.DeepCopy()
				orphan.SetName("udproute-unknown-namespace")
				orphan.SetNamespace("unknown-namespace")
				orphan.Spec.ParentRefs = []gwapiv1.ParentReference{{
					Name:        "gateway-1",
					Namespace:   new(gwapiv1.Namespace("testnamespace")),
					SectionName: new(gwapiv1.SectionName("no-match")),
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp, *orphan}
			},
			tester: func(t *testing.T, r *renderer) {
				rs := r.allRoutes()
				assert.Len(t, rs, 2, "route len")
				for _, ro := range rs {
					parents := ro.GetParentRefs()
					for i := range parents {
						exists, accepted := r.isParentAcceptingRoute(ro, &parents[i], "")
						assert.True(t, exists, "parent exists")
						assert.False(t, accepted, "parent rejects the route")
					}
				}
			},
		},
		{
			name:   "route statuses of TURNRoutes, unmasked Gateway API TCPRoutes and foreign parent kinds",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:    []gwapiv1.Gateway{testutils.TestGw},
			rs:     []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			turnrs: []stnrgwv1.TURNRoute{testutils.TestTURNRoute},
			tscrts: []corev1.Secret{testutils.TestTunnelSecret},
			svcs:   []corev1.Service{testutils.TestSvc},
			dps:    []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.ParentRefs = append(udp.Spec.ParentRefs, gwapiv1.ParentReference{
					Group: new(gwapiv1.Group("example.com")),
					Kind:  new(gwapiv1.Kind("Other")),
					Name:  "other",
				})
				c.rs = []stnrgwv1.UDPRoute{*udp}
				tr := testutils.TestTCPRoute.DeepCopy()
				tr.SetName("tcproute-gwapi")
				c.trsGwAPI = []stnrgwv1.TCPRoute{*tr}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))

				assert.Len(t, r.allRoutes(), 3, "the unmasked Gateway API TCPRoute is a route")
				assert.NoError(t, r.renderForGateways(c), "render success")

				tr := c.update.UpsertQueue.TURNRoutes.Get(store.GetNamespacedName(&testutils.TestTURNRoute))
				assert.NotNil(t, tr, "TURNRoute status")
				assert.NotNil(t, c.update.UpsertQueue.TCPRoutesGwAPI.Get(types.NamespacedName{
					Namespace: "testnamespace", Name: "tcproute-gwapi"}), "Gateway API TCPRoute status")

				ro := c.update.UpsertQueue.UDPRoutes.Get(store.GetNamespacedName(&testutils.TestUDPRoute)).(*stnrgwv1.UDPRoute)
				assert.Len(t, ro.Status.Parents, 2, "parent statuses")
				foreign := ro.Status.Parents[1].ParentRef
				assert.Equal(t, gwapiv1.Group("example.com"), *foreign.Group, "foreign group kept")
				assert.Equal(t, gwapiv1.Kind("Other"), *foreign.Kind, "foreign kind kept")
			},
		},
		{
			name: "an unknown Service type defaults, a missing Dataplane fails the render",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{opdefault.ServiceTypeAnnotationKey: "Bogus"})
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				gws := r.getGateways4Class(c)
				c.gws.ResetGateways(gws)

				s, _ := r.createLbService4Gateway(c, gws[0])
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "default type")

				err = r.renderForGateways(c)
				assert.True(t, IsCritical(err), "critical err")
				assert.Equal(t, "missing Dataplane resource for Gateway", err.Error(), "message")
			},
		},
		{
			name:  "a TLS listener with a certificate reference of a wrong kind",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			scrts: []corev1.Secret{testutils.TestSecret},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     "wrong-kind",
					Port:     2,
					Protocol: "TURN-TLS",
					TLS: &gwapiv1.ListenerTLSConfig{
						Mode: &mode,
						CertificateRefs: []gwapiv1.SecretObjectReference{{
							Kind: new(gwapiv1.Kind("ConfigMap")),
							Name: "testsecret-ok",
						}},
					},
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				gws := r.getGateways4Class(c)
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways(gws)

				for i := range gws[0].Spec.Listeners {
					l := gws[0].Spec.Listeners[i]
					lc, err := r.renderListener(c, &l, nil, gwAddrPort{addr: "1.2.3.4", port: 1234}, nil)
					assert.Nil(t, lc, "no listener for "+string(l.Name))
					assert.True(t, IsNonCriticalError(err, InvalidCertificateRef), "invalid cert ref")
					assert.Equal(t, "invalid certificate reference", err.Error(), "message")
				}
			},
		},
	})
}
