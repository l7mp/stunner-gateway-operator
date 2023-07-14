package renderer

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func TestRenderPipelineManagedMode(t *testing.T) {
	// managed mode
	renderTester(t, []renderTestConfig{
		{
			name: "no EDS - E2E test",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrv1a1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				config.DataplaneMode = config.DataplaneModeManaged

				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1a2.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta: configmap is now named after gateway!
				assert.Equal(t, o.GetName(), gw.GetName(),
					"configmap name")
				assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
					"configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, ok, "annotations: related gw")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarshal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auth.Realm, "realm")
				assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auth.Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auth.Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 1, "route num")
				assert.Equal(t, lc.Routes[0], "testnamespace/udproute-ok", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.testnamespace.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// gateway status
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
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1b1.GatewayReasonProgrammed),
					gw.Status.Conditions[1].Reason, "reason")

				// route status
				ros := store.UDPRoutes.GetAll()
				assert.Len(t, ros, 1, "routes len")
				ro := ros[0]
				p := ro.Spec.ParentRefs[0]

				assert.Len(t, ro.Status.Parents, 1, "parent status len")
				parentStatus := ro.Status.Parents[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1a2.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")

				// restore EDS
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		{
			name: "EDS with relay-to-cluster-IP - E2E test",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			dps:  []stnrv1a1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.DataplaneMode = config.DataplaneModeManaged

				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1a2.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta: configmap is now named after gateway!
				assert.Equal(t, o.GetName(), gw.GetName(),
					"configmap name")
				assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
					"configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, ok, "annotations: related gw")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auth.Realm, "realm")
				assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auth.Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auth.Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 1, "route num")
				assert.Equal(t, lc.Routes[0], "testnamespace/udproute-ok", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "4.3.2.1", "cluster-ip")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore EDS
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		{
			name: "E2E invalidation",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrv1a1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				config.DataplaneMode = config.DataplaneModeManaged

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1a2.Gateway{gw})

				r.invalidateGatewayClass(c, errors.New("dummy"))

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta: configmap is now named after gateway!
				assert.Equal(t, o.GetName(), gw.GetName(),
					"configmap name")
				assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
					"configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, ok, "annotations: related gw")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, found := cm.Data[opdefault.DefaultStunnerdConfigfileName]
				assert.True(t, found, "configmap data: stunnerd.conf found")
				assert.Equal(t, "", conf, "configmap data: stunnerd.conf empty")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				//statuses
				setGatewayClassStatusAccepted(gc, nil)
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				assert.Equal(t, string(gwapiv1b1.GatewayClassConditionStatusAccepted),
					gc.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, metav1.ConditionTrue,
					gc.Status.Conditions[0].Status, "conditions status")
				assert.Equal(t, string(gwapiv1b1.GatewayClassReasonAccepted),
					gc.Status.Conditions[0].Type, "conditions reason")
				assert.Equal(t, int64(0),
					gc.Status.Conditions[0].ObservedGeneration, "conditions gen")

				objs := c.update.UpsertQueue.Gateways.Objects()
				assert.Len(t, gws, 1, "gateway num")
				gw, found = objs[0].(*gwapiv1a2.Gateway)
				assert.True(t, found, "gateway found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

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
			name: "EDS with no relay-to-cluster-IP - E2E rendering for multiple gateway-classes",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			dps:  []stnrv1a1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				// a new gatewayclass that specifies a different gateway-config
				// a new gatewayclass that specifies a different gateway-config
				dummyGc := testutils.TestGwClass.DeepCopy()
				dummyGc.SetName("dummy-gateway-class")
				dummyGc.Spec.ParametersRef = &gwapiv1a2.ParametersReference{
					Group:     gwapiv1a2.Group(stnrv1a1.GroupVersion.Group),
					Kind:      gwapiv1a2.Kind("GatewayConfig"),
					Name:      "dummy-gateway-config",
					Namespace: &testutils.TestNsName,
				}
				c.cls = []gwapiv1a2.GatewayClass{testutils.TestGwClass, *dummyGc}

				// the new gateway-config that renders into a different stunner configmap
				dummyConf := testutils.TestGwConfig.DeepCopy()
				dummyConf.SetName("dummy-gateway-config")
				target := "dummy-stunner-config"
				dummyConf.Spec.StunnerConfig = &target
				c.cfs = []stnrv1a1.GatewayConfig{testutils.TestGwConfig, *dummyConf}

				// a new gateway whose controller-name is the new gatewayclass
				dummyGw := testutils.TestGw.DeepCopy()
				dummyGw.SetName("dummy-gateway")
				dummyGw.Spec.GatewayClassName =
					gwapiv1a2.ObjectName("dummy-gateway-class")
				c.gws = []gwapiv1a2.Gateway{*dummyGw, testutils.TestGw}

				// a route for dummy-gateway
				sn := gwapiv1a2.SectionName("gateway-1-listener-udp")
				udpRoute := testutils.TestUDPRoute.DeepCopy()
				udpRoute.Spec.CommonRouteSpec.ParentRefs = []gwapiv1a2.ParentReference{{
					Name:      "dummy-gateway",
					Namespace: &testutils.TestNsName,
				}, {
					Name:        gwapiv1a2.ObjectName(testutils.TestGw.GetName()),
					Namespace:   &testutils.TestNsName,
					SectionName: &sn,
				}}
				udpRoute.Spec.Rules[0].BackendRefs = []gwapiv1a2.BackendRef{{
					BackendObjectReference: gwapiv1a2.BackendObjectReference{
						Name:      gwapiv1a2.ObjectName(testutils.TestSvc.GetName()),
						Namespace: &testutils.TestNsName,
					},
				}, {
					BackendObjectReference: gwapiv1a2.BackendObjectReference{
						Name:      "dummy-service",
						Namespace: &testutils.TestNsName,
					},
				}}
				c.rs = []gwapiv1a2.UDPRoute{*udpRoute}

				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				dummySvc := testutils.TestSvc.DeepCopy()
				dummySvc.SetName("dummy-service")
				c.svcs = []corev1.Service{*s, *dummySvc}

				dummyEp := testutils.TestEndpoint.DeepCopy()
				dummyEp.SetName("dummy-service")
				dummyEp.Subsets = []corev1.EndpointSubset{{
					Addresses:         []corev1.EndpointAddress{{IP: "4.4.4.4"}},
					NotReadyAddresses: []corev1.EndpointAddress{{}},
				}}
				c.eps = []corev1.Endpoints{testutils.TestEndpoint, *dummyEp}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.DataplaneMode = config.DataplaneModeManaged

				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = false

				gcs := r.getGatewayClasses()
				assert.Len(t, gcs, 2, "gw-classes found")

				// original config
				gc := gcs[0]
				// we can never know the order...
				if gc.GetName() == "dummy-gateway-class" {
					gc = gcs[1]
				}

				assert.Equal(t, "gatewayclass-ok", gc.GetName(),
					"gatewayclass name")

				var err error
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")

				// render first gw
				gw := gws[0]
				c.gws.ResetGateways([]*gwapiv1a2.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o := cms[0]

				// objectmeta: configmap is now named after gateway!
				assert.Equal(t, o.GetName(), gw.GetName(),
					"configmap name")
				assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
					"configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, ok, "annotations: related gw")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auth.Realm, "realm")
				assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auth.Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auth.Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 1, "route num")
				assert.Equal(t, lc.Routes[0], "testnamespace/udproute-ok", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-5")

				// gateway status
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
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1b1.GatewayReasonProgrammed),
					gw.Status.Conditions[1].Reason, "reason")

				// route status
				ros := store.UDPRoutes.GetAll()
				assert.Len(t, ros, 1, "routes len")
				ro := ros[0]

				assert.Len(t, ro.Status.Parents, 2, "parent status len")
				parentStatus := ro.Status.Parents[0]
				p := ro.Spec.ParentRefs[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1a2.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")

				parentStatus = ro.Status.Parents[1]
				p = ro.Spec.ParentRefs[1]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1a2.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// config for the modified gateway-class
				gc = gcs[1]
				// we can never know the order...
				if gc.GetName() != "dummy-gateway-class" {
					gc = gcs[0]
				}
				assert.Equal(t, "dummy-gateway-class", gc.GetName(),
					"gatewayclass name")

				c = &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "dummy-gateway-config", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws = r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")

				// render gw
				gw = gws[0]
				c.gws.ResetGateways([]*gwapiv1a2.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms = c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o = cms[0]

				// objectmeta: configmap is now named after gateway!
				assert.Equal(t, o.GetName(), gw.GetName(),
					"configmap name")
				assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
					"configmap namespace")

				// related gw
				as = o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok = as[opdefault.RelatedGatewayAnnotationKey]
				assert.True(t, ok, "annotations: related gw")

				cm, ok = o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err = store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auth.Realm, "realm")
				assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auth.Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auth.Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc = conf.Listeners[0]
				assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				// the service links to the original gateway, our gateway does not
				// have linkage, so public addr should be empty
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 1, "route num")
				assert.Equal(t, lc.Routes[0], "testnamespace/udproute-ok", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 1, "route num")
				assert.Equal(t, lc.Routes[0], "testnamespace/udproute-ok", "udp route")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc = conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-5")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// gateway status
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
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1b1.GatewayReasonProgrammed),
					gw.Status.Conditions[1].Reason, "reason")

				// route status
				assert.Len(t, ro.Status.Parents, 2, "parent status len")
				parentStatus = ro.Status.Parents[0]
				p = ro.Spec.ParentRefs[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1a2.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")

				parentStatus = ro.Status.Parents[1]
				p = ro.Spec.ParentRefs[1]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1a2.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1a2.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")

				// restore EDS
				config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		// {
		// 	name: "EDS with relay-to-cluster-IP - E2E rendering for multiple gateway-classes",
		// 	cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
		// 	cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
		// 	gws:  []gwapiv1a2.Gateway{testutils.TestGw},
		// 	rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
		// 	svcs: []corev1.Service{testutils.TestSvc},
		// 	eps:  []corev1.Endpoints{testutils.TestEndpoint},
		// 	dps:  []stnrv1a1.Dataplane{testutils.TestDataplane},
		// 	prep: func(c *renderTestConfig) {
		// 		// a new gatewayclass that specifies a different gateway-config
		// 		dummyGc := testutils.TestGwClass.DeepCopy()
		// 		dummyGc.SetName("dummy-gateway-class")
		// 		dummyGc.Spec.ParametersRef = &gwapiv1a2.ParametersReference{
		// 			Group:     gwapiv1a2.Group(stnrv1a1.GroupVersion.Group),
		// 			Kind:      gwapiv1a2.Kind("GatewayConfig"),
		// 			Name:      "dummy-gateway-config",
		// 			Namespace: &testutils.TestNsName,
		// 		}
		// 		c.cls = []gwapiv1a2.GatewayClass{testutils.TestGwClass, *dummyGc}

		// 		// the new gateway-config that renders into a different stunner configmap
		// 		dummyConf := testutils.TestGwConfig.DeepCopy()
		// 		dummyConf.SetName("dummy-gateway-config")
		// 		target := "dummy-stunner-config"
		// 		dummyConf.Spec.StunnerConfig = &target
		// 		c.cfs = []stnrv1a1.GatewayConfig{testutils.TestGwConfig, *dummyConf}

		// 		// a new gateway whose controller-name is the new gatewayclass
		// 		dummyGw := testutils.TestGw.DeepCopy()
		// 		dummyGw.SetName("dummy-gateway")
		// 		dummyGw.Spec.GatewayClassName =
		// 			gwapiv1a2.ObjectName("dummy-gateway-class")
		// 		c.gws = []gwapiv1a2.Gateway{*dummyGw, testutils.TestGw}

		// 		// a route for dummy-gateway
		// 		dummyUdp := testutils.TestUDPRoute.DeepCopy()
		// 		dummyUdp.SetName("dummy-route")
		// 		dummyUdp.Spec.CommonRouteSpec.ParentRefs[0].Name = "dummy-gateway"
		// 		dummyUdp.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
		// 			gwapiv1a2.ObjectName("dummy-service")
		// 		c.rs = []gwapiv1a2.UDPRoute{*dummyUdp, testutils.TestUDPRoute}

		// 		s := testutils.TestSvc.DeepCopy()
		// 		s.Spec.ClusterIP = "4.3.2.1"
		// 		dummySvc := testutils.TestSvc.DeepCopy()
		// 		dummySvc.SetName("dummy-service")
		// 		c.svcs = []corev1.Service{*s, *dummySvc}

		// 		dummyEp := testutils.TestEndpoint.DeepCopy()
		// 		dummyEp.SetName("dummy-service")
		// 		dummyEp.Subsets = []corev1.EndpointSubset{{
		// 			Addresses:         []corev1.EndpointAddress{{IP: "4.4.4.4"}},
		// 			NotReadyAddresses: []corev1.EndpointAddress{{}},
		// 		}}
		// 		c.eps = []corev1.Endpoints{testutils.TestEndpoint, *dummyEp}
		// 	},
		// 	tester: func(t *testing.T, r *Renderer) {
		// 		config.DataplaneMode = config.DataplaneModeManaged

		// 		config.EnableEndpointDiscovery = true
		// 		config.EnableRelayToClusterIP = true

		// 		gcs := r.getGatewayClasses()
		// 		assert.Len(t, gcs, 2, "gw-classes found")

		// 		// original config
		// 		gc := gcs[0]
		// 		// we can never know the order...
		// 		if gc.GetName() == "dummy-gateway-class" {
		// 			gc = gcs[1]
		// 		}

		// 		assert.Equal(t, "gatewayclass-ok", gc.GetName(),
		// 			"gatewayclass name")

		// 		c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
		// 		c.update = event.NewEventUpdate(0)
		// 		assert.NotNil(t, c.update, "update event create")

		// 		c.gws.ResetGateways(r.getGateways4Class(c))
		// 		err := r.renderForGateways(c)
		// 		assert.NoError(t, err, "render success")

		// 		// configmap
		// 		cms := c.update.UpsertQueue.ConfigMaps.Objects()
		// 		assert.Len(t, cms, 1, "configmap ready")

		// 		o := cms[0]

		// 		// objectmeta: configmap is now named after gateway!
		// 		assert.Equal(t, o.GetName(), gw.GetName(),
		// 			"configmap name")
		// 		assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
		// 			"configmap namespace")

		// 		// related gw
		// 		as := o.GetAnnotations()
		// 		assert.Len(t, as, 1, "annotations len")
		// 		_, ok := as[opdefault.RelatedGatewayAnnotationKey]
		// 		assert.True(t, ok, "annotations: related gw")

		// 		cm, ok := o.(*corev1.ConfigMap)
		// 		assert.True(t, ok, "configmap cast")

		// 		conf, err := store.UnpackConfigMap(cm)
		// 		assert.NoError(t, err, "configmap stunner-config unmarschal")

		// 		assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
		// 			conf.Admin.Name, "name")
		// 		assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
		// 			"loglevel")

		// 		assert.Equal(t, testutils.TestRealm, conf.Auth.Realm, "realm")
		// 		assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
		// 		assert.Equal(t, testutils.TestUsername, conf.Auth.Credentials["username"],
		// 			"username")
		// 		assert.Equal(t, testutils.TestPassword, conf.Auth.Credentials["password"],
		// 			"password")

		// 		assert.Len(t, conf.Listeners, 2, "listener num")
		// 		lc := conf.Listeners[0]
		// 		assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
		// 		assert.Equal(t, "UDP", lc.Protocol, "proto")
		// 		assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
		// 		assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
		// 		assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
		// 		assert.Len(t, lc.Routes, 1, "route num")
		// 		assert.Equal(t, lc.Routes[0], "testnamespace/udproute-ok", "udp route")

		// 		lc = conf.Listeners[1]
		// 		assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
		// 		assert.Equal(t, "TCP", lc.Protocol, "proto")
		// 		assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
		// 		assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
		// 		assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
		// 		assert.Len(t, lc.Routes, 0, "route num")

		// 		assert.Len(t, conf.Clusters, 1, "cluster num")
		// 		rc := conf.Clusters[0]
		// 		assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
		// 		assert.Equal(t, "STATIC", rc.Type, "cluster type")
		// 		assert.Len(t, rc.Endpoints, 5, "endpoints len")
		// 		assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
		// 		assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
		// 		assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
		// 		assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
		// 		assert.Contains(t, rc.Endpoints, "4.3.2.1", "cluster-ip")

		// 		// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

		// 		// config for the modified gateway-class
		// 		gc = gcs[1]
		// 		// we can never know the order...
		// 		if gc.GetName() != "dummy-gateway-class" {
		// 			gc = gcs[0]
		// 		}

		// 		assert.Equal(t, "dummy-gateway-class", gc.GetName(),
		// 			"gatewayclass name")

		// 		c = &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
		// 		c.update = event.NewEventUpdate(0)
		// 		assert.NotNil(t, c.update, "update event create")

		// 		c.gws.ResetGateways(r.getGateways4Class(c))
		// 		err = r.renderForGateways(c)
		// 		assert.NoError(t, err, "render success")

		// 		// configmap
		// 		cms = c.update.UpsertQueue.ConfigMaps.Objects()
		// 		assert.Len(t, cms, 1, "configmap ready")

		// 		o = cms[0]

		// 		// objectmeta: configmap is now named after gateway!
		// 		assert.Equal(t, o.GetName(), gw.GetName(),
		// 			"configmap name")
		// 		assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
		// 			"configmap namespace")

		// 		// related gw
		// 		as = o.GetAnnotations()
		// 		assert.Len(t, as, 1, "annotations len")
		// 		_, ok = as[opdefault.RelatedGatewayAnnotationKey]
		// 		assert.True(t, ok, "annotations: related gw")

		// 		cm, ok = o.(*corev1.ConfigMap)
		// 		assert.True(t, ok, "configmap cast")

		// 		conf, err = store.UnpackConfigMap(cm)
		// 		assert.NoError(t, err, "configmap stunner-config unmarschal")

		// 		assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
		// 			conf.Admin.Name, "name")
		// 		assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
		// 			"loglevel")

		// 		assert.Equal(t, testutils.TestRealm, conf.Auth.Realm, "realm")
		// 		assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
		// 		assert.Equal(t, testutils.TestUsername, conf.Auth.Credentials["username"],
		// 			"username")
		// 		assert.Equal(t, testutils.TestPassword, conf.Auth.Credentials["password"],
		// 			"password")

		// 		assert.Len(t, conf.Listeners, 2, "listener num")
		// 		lc = conf.Listeners[0]
		// 		assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-udp", lc.Name, "name")
		// 		assert.Equal(t, "UDP", lc.Protocol, "proto")
		// 		// the service links to the original gateway, our gateway does not
		// 		// have linkage, so public addr should be empty
		// 		assert.Equal(t, "", lc.PublicAddr, "public-ip")
		// 		assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
		// 		assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
		// 		assert.Len(t, lc.Routes, 1, "route num")
		// 		assert.Equal(t, lc.Routes[0], "testnamespace/dummy-route", "udp route")

		// 		lc = conf.Listeners[1]
		// 		assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
		// 		assert.Equal(t, "TCP", lc.Protocol, "proto")
		// 		assert.Equal(t, "", lc.PublicAddr, "public-ip")
		// 		assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
		// 		assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
		// 		assert.Len(t, lc.Routes, 0, "route num")

		// 		assert.Len(t, conf.Clusters, 1, "cluster num")
		// 		rc = conf.Clusters[0]
		// 		assert.Equal(t, "testnamespace/dummy-route", rc.Name, "cluster name")
		// 		assert.Equal(t, "STATIC", rc.Type, "cluster type")
		// 		assert.Len(t, rc.Endpoints, 1, "endpoints len")
		// 		assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-1")

		// 		// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

		// 		// restore EDS
		// 		config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
		// 		config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
		// 		config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
		// 	},
		// },
		// {
		// 	name:  "StaticService - E2E test",
		// 	cls:   []gwapiv1a2.GatewayClass{testutils.TestGwClass},
		// 	cfs:   []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
		// 	gws:   []gwapiv1a2.Gateway{testutils.TestGw},
		// 	svcs:  []corev1.Service{testutils.TestSvc},
		// 	ssvcs: []stnrv1a1.StaticService{testutils.TestStaticSvc},
		// 	dps:   []stnrv1a1.Dataplane{testutils.TestDataplane},
		// 	prep: func(c *renderTestConfig) {
		// 		group := gwapiv1a2.Group(stnrv1a1.GroupVersion.Group)
		// 		kind := gwapiv1a2.Kind("StaticService")
		// 		udp := testutils.TestUDPRoute.DeepCopy()
		// 		udp.Spec.Rules[0].BackendRefs = []gwapiv1a2.BackendRef{{
		// 			BackendObjectReference: gwapiv1a2.BackendObjectReference{
		// 				Group: &group,
		// 				Kind:  &kind,
		// 				Name:  "teststaticservice-ok",
		// 			},
		// 		}}
		// 		c.rs = []gwapiv1a2.UDPRoute{*udp}
		// 	},
		// 	tester: func(t *testing.T, r *Renderer) {
		// 		config.DataplaneMode = config.DataplaneModeManaged

		// 		gc, err := r.getGatewayClass()
		// 		assert.NoError(t, err, "gw-class found")
		// 		c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
		// 		c.gwConf, err = r.getGatewayConfig4Class(c)
		// 		assert.NoError(t, err, "gw-conf found")
		// 		assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
		// 			"gatewayconfig name")

		// 		c.update = event.NewEventUpdate(0)
		// 		assert.NotNil(t, c.update, "update event create")

		// 		c.gws.ResetGateways(r.getGateways4Class(c))
		// 		err = r.renderForGateways(c)
		// 		assert.NoError(t, err, "render success")

		// 		// configmap
		// 		cms := c.update.UpsertQueue.ConfigMaps.Objects()
		// 		assert.Len(t, cms, 1, "configmap ready")
		// 		o := cms[0]

		// 		// objectmeta: configmap is now named after gateway!
		// 		assert.Equal(t, o.GetName(), gw.GetName(),
		// 			"configmap name")
		// 		assert.Equal(t, o.GetNamespace(), gw.GetNamespace(),
		// 			"configmap namespace")

		// 		// related gw
		// 		as := o.GetAnnotations()
		// 		assert.Len(t, as, 1, "annotations len")
		// 		_, ok := as[opdefault.RelatedGatewayAnnotationKey]
		// 		assert.True(t, ok, "annotations: related gw")

		// 		cm, ok := o.(*corev1.ConfigMap)
		// 		assert.True(t, ok, "configmap cast")

		// 		conf, err := store.UnpackConfigMap(cm)
		// 		assert.NoError(t, err, "configmap stunner-config unmarshal")

		// 		assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
		// 			conf.Admin.Name, "name")
		// 		assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
		// 			"loglevel")

		// 		assert.Equal(t, testutils.TestRealm, conf.Auth.Realm, "realm")
		// 		assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
		// 		assert.Equal(t, testutils.TestUsername, conf.Auth.Credentials["username"],
		// 			"username")
		// 		assert.Equal(t, testutils.TestPassword, conf.Auth.Credentials["password"],
		// 			"password")

		// 		assert.Len(t, conf.Listeners, 2, "listener num")
		// 		lc := conf.Listeners[0]
		// 		assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
		// 		assert.Equal(t, "UDP", lc.Protocol, "proto")
		// 		assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
		// 		assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
		// 		assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
		// 		assert.Len(t, lc.Routes, 1, "route num")
		// 		assert.Equal(t, lc.Routes[0], "testnamespace/udproute-ok", "udp route")

		// 		lc = conf.Listeners[1]
		// 		assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
		// 		assert.Equal(t, "TCP", lc.Protocol, "proto")
		// 		assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
		// 		assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
		// 		assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
		// 		assert.Len(t, lc.Routes, 0, "route num")

		// 		assert.Len(t, conf.Clusters, 1, "cluster num")
		// 		rc := conf.Clusters[0]
		// 		assert.Equal(t, "testnamespace/udproute-ok", rc.Name, "cluster name")
		// 		assert.Equal(t, "STATIC", rc.Type, "cluster type")
		// 		assert.Len(t, rc.Endpoints, 3, "endpoints len")
		// 		assert.Contains(t, rc.Endpoints, "10.11.12.13", "staticservice endpoint ip-1")
		// 		assert.Contains(t, rc.Endpoints, "10.11.12.14", "staticservice endpoint ip-2")
		// 		assert.Contains(t, rc.Endpoints, "10.11.12.15", "staticservice endpoint ip-3")

		// 		//statuses
		// 		setGatewayClassStatusAccepted(gc, nil)
		// 		assert.Len(t, gc.Status.Conditions, 1, "conditions num")
		// 		assert.Equal(t, string(gwapiv1b1.GatewayClassConditionStatusAccepted),
		// 			gc.Status.Conditions[0].Type, "conditions accepted")
		// 		assert.Equal(t, metav1.ConditionTrue,
		// 			gc.Status.Conditions[0].Status, "conditions status")
		// 		assert.Equal(t, string(gwapiv1b1.GatewayClassReasonAccepted),
		// 			gc.Status.Conditions[0].Type, "conditions reason")
		// 		assert.Equal(t, int64(0),
		// 			gc.Status.Conditions[0].ObservedGeneration, "conditions gen")

		// 		gws := c.update.UpsertQueue.Gateways.Objects()
		// 		assert.Len(t, gws, 1, "gateway num")
		// 		gw, found := gws[0].(*gwapiv1a2.Gateway)
		// 		assert.True(t, found, "gateway found")
		// 		assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
		// 			store.GetObjectKey(gw), "gw name found")

		// 		assert.Len(t, gw.Status.Conditions, 2, "conditions num")

		// 		assert.Equal(t, string(gwapiv1b1.GatewayConditionAccepted),
		// 			gw.Status.Conditions[0].Type, "conditions accepted")
		// 		assert.Equal(t, int64(0), gw.Status.Conditions[0].ObservedGeneration,
		// 			"conditions gen")
		// 		assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[0].Status,
		// 			"status")
		// 		assert.Equal(t, string(gwapiv1b1.GatewayReasonAccepted),
		// 			gw.Status.Conditions[0].Reason, "reason")

		// 		assert.Equal(t, string(gwapiv1b1.GatewayConditionProgrammed),
		// 			gw.Status.Conditions[1].Type, "programmed")
		// 		assert.Equal(t, int64(0), gw.Status.Conditions[1].ObservedGeneration,
		// 			"conditions gen")
		// 		assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[1].Status,
		// 			"status")
		// 		assert.Equal(t, string(gwapiv1b1.GatewayReasonProgrammed),
		// 			gw.Status.Conditions[1].Reason, "reason")

		// 		ros := c.update.UpsertQueue.UDPRoutes.Objects()
		// 		assert.Len(t, ros, 1, "routenum")
		// 		ro, found := ros[0].(*gwapiv1a2.UDPRoute)
		// 		assert.True(t, found, "route found")
		// 		assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-ok"),
		// 			store.GetObjectKey(ro), "route name found")

		// 		assert.Len(t, ro.Status.Parents, 1, "parent status len")
		// 		p := ro.Spec.ParentRefs[0]
		// 		parentStatus := ro.Status.Parents[0]

		// 		assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
		// 		assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
		// 		assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
		// 		assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
		// 		assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

		// 		assert.Equal(t, gwapiv1a2.GatewayController("stunner.l7mp.io/gateway-operator"),
		// 			parentStatus.ControllerName, "status parent ref")

		// 		d := meta.FindStatusCondition(parentStatus.Conditions,
		// 			string(gwapiv1a2.RouteConditionAccepted))
		// 		assert.NotNil(t, d, "accepted found")
		// 		assert.Equal(t, string(gwapiv1a2.RouteConditionAccepted), d.Type,
		// 			"type")
		// 		assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
		// 		assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
		// 		assert.Equal(t, "Accepted", d.Reason, "reason")

		// 		d = meta.FindStatusCondition(parentStatus.Conditions,
		// 			string(gwapiv1a2.RouteConditionResolvedRefs))
		// 		assert.NotNil(t, d, "resolved-refs found")
		// 		assert.Equal(t, string(gwapiv1a2.RouteConditionResolvedRefs), d.Type,
		// 			"type")
		// 		assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
		// 		assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
		// 		assert.Equal(t, "ResolvedRefs", d.Reason, "reason")

		// 		config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
		// 	},
		// },
	})
}
