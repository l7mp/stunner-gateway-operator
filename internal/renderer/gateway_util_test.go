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

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
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
		{
			name: "gateway status ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				initGatewayStatus(gw, nil)
				setGatewayStatusProgrammed(gw, errors.New("dummy"), nil)
				assert.Len(t, gw.Status.Addresses, 0, "status addresses")

				assert.Len(t, gw.Status.Conditions, 2, "conditions num")

				assert.Equal(t, string(gwapiv1.GatewayConditionAccepted),
					gw.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, int64(0), gw.Status.Conditions[0].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonAccepted),
					gw.Status.Conditions[0].Reason, "reason")

				assert.Equal(t, string(gwapiv1.GatewayConditionProgrammed),
					gw.Status.Conditions[1].Type, "programmed")
				assert.Equal(t, int64(0), gw.Status.Conditions[1].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionFalse, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonInvalid),
					gw.Status.Conditions[1].Reason, "reason")
			},
		},
		{
			name: "gateway rescheduled/re-ready status ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				initGatewayStatus(gw, nil)
				setGatewayStatusProgrammed(gw, errors.New("dummy"), nil)
				gw.ObjectMeta.SetGeneration(1)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class not found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				initGatewayStatus(gw, nil)
				setGatewayStatusProgrammed(gw, errors.New("dummy"), nil)
				assert.Len(t, gw.Status.Addresses, 0, "status addresses")

				assert.Len(t, gw.Status.Conditions, 2, "conditions num")

				assert.Equal(t, string(gwapiv1.GatewayConditionAccepted),
					gw.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, int64(1), gw.Status.Conditions[0].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonAccepted),
					gw.Status.Conditions[0].Reason, "reason")

				assert.Equal(t, string(gwapiv1.GatewayConditionProgrammed),
					gw.Status.Conditions[1].Type, "programmed")
				assert.Equal(t, int64(1), gw.Status.Conditions[1].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionFalse, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonInvalid),
					gw.Status.Conditions[1].Reason, "reason")
			},
		},
		{
			name: "listener status ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, store.GetObjectKey(gw), fmt.Sprintf("%s/%s",
					testutils.TestNsName, "gateway-1"), "gw name found")
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				initGatewayStatus(gw, nil)
				conflicted := false
				for j := range gw.Spec.Listeners {
					l := gw.Spec.Listeners[j]
					addr := gwAddrPort{
						addr: "1.2.3.4",
						port: 1234,
					}

					_, err := r.renderListener(c, &l, []store.Route{}, addr, nil)

					if err != nil {
						setListenerStatus(gw, &l, err, conflicted, 0)
						continue
					}

					setListenerStatus(gw, &l, nil, conflicted, 2)
				}

				// listeners[0]: ok
				assert.Len(t, gw.Status.Listeners, 2, "conditions num")
				d := meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonNoConflicts),
					d.Reason, "reason")

				// listeners[1]: ok
				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonNoConflicts),
					d.Reason, "reason")
			},
		},
		{
			name: "listener status invalid cert-ref",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.ListenerTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("missing-secret"),
					}},
				}

				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"),
					TLS:      &tls,
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
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]

				initGatewayStatus(gw, nil)
				l := gw.Spec.Listeners[0]
				setListenerStatus(gw, &l, NewNonCriticalError(InvalidCertificateRef), false, 0)

				assert.Len(t, gw.Status.Listeners, 1, "conditions num")

				d := meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted), d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resolvedrefs found")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, string(gwapiv1.ListenerReasonInvalidCertificateRef),
					d.Reason, "reason")

				// an SRT listener is not accepted, for an unsupported protocol
				setListenerStatus(gw, &l, NewNonCriticalError(UnsupportedSRT), false, 0)
				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, string(gwapiv1.ListenerReasonUnsupportedProtocol), d.Reason, "reason")
				assert.Equal(t, "SRT listeners are not supported", d.Message, "message")
			},
		},
		{
			name: "invalid listener status",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("invalid"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("dummy"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-tcp"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-TCP"),
				}}
				initGatewayStatus(gw, nil)
				setGatewayStatusProgrammed(gw, errors.New("dummy"), nil)
				gw.ObjectMeta.SetGeneration(1)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gw found")
				gw := gws[0]
				assert.Equal(t, store.GetObjectKey(gw), fmt.Sprintf("%s/%s",
					testutils.TestNsName, "gateway-1"), "gw name found")
				c.gws = store.NewGatewayStore()
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				initGatewayStatus(gw, nil)
				conflicted := false
				for j := range gw.Spec.Listeners {
					l := gw.Spec.Listeners[j]
					addr := gwAddrPort{
						addr: "1.2.3.4",
						port: 1234,
					}

					_, err := r.renderListener(c, &l, []store.Route{}, addr, nil)

					if err != nil {
						setListenerStatus(gw, &l, err, conflicted, 0)
						continue
					}

					setListenerStatus(gw, &l, nil, conflicted, 2)
				}

				// listeners[0]: ok
				assert.Len(t, gw.Status.Listeners, 3, "conditions num")
				d := meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonNoConflicts),
					d.Reason, "reason")

				// listeners[1]: detached
				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonUnsupportedProtocol),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonNoConflicts),
					d.Reason, "reason")

				// listeners[2]: ok
				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(1), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonNoConflicts),
					d.Reason, "reason")
			},
		},
	})
}

// TestListenerSupportedKinds pins that a listener supports the optional TURNRoute only when the
// cluster serves its CRD.
func TestListenerSupportedKinds(t *testing.T) {
	defer func(served bool) { config.TURNRouteServed = served }(config.TURNRouteServed)
	kinds := func() []string {
		gw := testutils.TestGw.DeepCopy()
		initGatewayStatus(gw, nil)
		ret := []string{}
		for _, k := range gw.Status.Listeners[0].SupportedKinds {
			ret = append(ret, string(k.Kind))
		}
		return ret
	}

	config.TURNRouteServed = false
	assert.NotContains(t, kinds(), "TURNRoute", "CRD not served")
	config.TURNRouteServed = true
	assert.Contains(t, kinds(), "TURNRoute", "CRD served")
}

func TestGatewayStatusProgrammedRollout(t *testing.T) {
	defer store.Deployments.Flush()
	gw := testutils.TestGw.DeepCopy()
	programmed := func() *metav1.Condition {
		setGatewayStatusProgrammed(gw, nil, []gwAddrPort{{addr: "1.2.3.4", port: 1}})
		return meta.FindStatusCondition(gw.Status.Conditions, string(gwapiv1.GatewayConditionProgrammed))
	}

	assert.Equal(t, metav1.ConditionTrue, programmed().Status, "no Deployment yet")

	one := int32(1)
	dep := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: gw.GetNamespace(), Name: gw.GetName()},
		Spec: appv1.DeploymentSpec{Replicas: &one}}
	dep.Status.Conditions = []appv1.DeploymentCondition{{Type: appv1.DeploymentProgressing,
		Status: corev1.ConditionTrue, Reason: "ReplicaSetUpdated"}}
	store.Deployments.Reset([]client.Object{dep})
	assert.Equal(t, metav1.ConditionTrue, programmed().Status, "a rollout in progress is ready soon")

	dep.Status.Conditions[0] = appv1.DeploymentCondition{Type: appv1.DeploymentProgressing,
		Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"}
	store.Deployments.Reset([]client.Object{dep})
	c := programmed()
	assert.Equal(t, metav1.ConditionFalse, c.Status, "a stalled rollout")
	assert.Equal(t, string(gwapiv1.GatewayReasonPending), c.Reason)

	dep.Status.Conditions = []appv1.DeploymentCondition{{Type: appv1.DeploymentReplicaFailure,
		Status: corev1.ConditionTrue, Message: "exceeded quota"}}
	store.Deployments.Reset([]client.Object{dep})
	c = programmed()
	assert.Equal(t, metav1.ConditionFalse, c.Status, "pods cannot be created")
	assert.Contains(t, c.Message, "exceeded quota")

	gw.SetAnnotations(map[string]string{opdefault.ManagedDataplaneDisabledAnnotationKey: opdefault.ManagedDataplaneDisabledAnnotationValue})
	assert.Equal(t, metav1.ConditionTrue, programmed().Status, "no managed dataplane")
}
