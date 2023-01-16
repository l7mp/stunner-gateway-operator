package renderer

import (
	// "context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderPipeline(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "piecewise render",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "admin rendered")
				assert.Equal(t, "testloglevel", admin.LogLevel, "log level")
				assert.Equal(t, config.DefaultStunnerdInstanceName,
					admin.Name, "stunnerd name")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "auth rendered")
				assert.Equal(t, stnrconfv1a1.AuthTypePlainText.String(),
					auth.Type, "auth type")
				assert.Equal(t, "testrealm", auth.Realm, "realm")
				assert.Equal(t, "testuser", auth.Credentials["username"], "username")
				assert.Equal(t, "testpass", auth.Credentials["password"], "password")
			},
		},
		{
			name: "no EDS - E2E test",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				// switch EDS off
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err = r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testutils.TestStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarshal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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

				// restore EDS
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "EDS without relay-to-cluster-IP - E2E test",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err = r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testutils.TestStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore EDS
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
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
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err = r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testutils.TestStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "E2E invalidation",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				r.invalidateGatewayClass(c, errors.New("dummy"))

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testutils.TestStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, found := cm.Data[config.DefaultStunnerdConfigfileName]
				assert.True(t, found, "configmap data: stunnerd.conf found")
				assert.Equal(t, "", conf, "configmap data: stunnerd.conf empty")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				//statuses
				setGatewayClassStatusAccepted(gc, nil)
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				assert.Equal(t, string(gwapiv1a2.GatewayClassConditionStatusAccepted),
					gc.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, metav1.ConditionTrue,
					gc.Status.Conditions[0].Status, "conditions status")
				assert.Equal(t, string(gwapiv1a2.GatewayClassReasonAccepted),
					gc.Status.Conditions[0].Type, "conditions reason")
				assert.Equal(t, int64(0),
					gc.Status.Conditions[0].ObservedGeneration, "conditions gen")

				gws := c.update.UpsertQueue.Gateways.Objects()
				assert.Len(t, gws, 1, "gateway num")
				gw, found := gws[0].(*gwapiv1a2.Gateway)
				assert.True(t, found, "gateway found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNs, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				assert.Len(t, gw.Status.Conditions, 2, "conditions num")
				assert.Equal(t, string(gwapiv1a2.GatewayConditionScheduled),
					gw.Status.Conditions[0].Type, "conditions sched")
				assert.Equal(t, metav1.ConditionTrue,
					gw.Status.Conditions[0].Status, "status ready")
				assert.Equal(t, int64(0),
					gw.Status.Conditions[0].ObservedGeneration, "conditions gen")
				assert.Equal(t, string(gwapiv1a2.GatewayConditionReady),
					gw.Status.Conditions[1].Type, "conditions ready")
				assert.Equal(t, metav1.ConditionFalse,
					gw.Status.Conditions[1].Status, "status ready")
				assert.Equal(t, int64(0),
					gw.Status.Conditions[1].ObservedGeneration, "conditions gen")
			},
		},
		{
			name: "no EDS - E2E rendering for multiple gateway-classes",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				config.EnableEndpointDiscovery = false
				config.EnableRelayToClusterIP = false

				// a new gatewayclass that specifies a different gateway-config
				dummyGc := testutils.TestGwClass.DeepCopy()
				dummyGc.SetName("dummy-gateway-class")
				dummyGc.Spec.ParametersRef = &gwapiv1a2.ParametersReference{
					Group:     gwapiv1a2.Group(stnrv1a1.GroupVersion.Group),
					Kind:      gwapiv1a2.Kind("GatewayConfig"),
					Name:      "dummy-gateway-config",
					Namespace: &testutils.TestNs,
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
				dummyUdp := testutils.TestUDPRoute.DeepCopy()
				dummyUdp.SetName("dummy-route")
				dummyUdp.Spec.CommonRouteSpec.ParentRefs[0].Name = "dummy-gateway"
				dummyUdp.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gwapiv1a2.ObjectName("dummy-service")
				c.rs = []gwapiv1a2.UDPRoute{*dummyUdp, testutils.TestUDPRoute}

				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				dummySvc := testutils.TestSvc.DeepCopy()
				dummySvc.SetName("dummy-service")
				c.svcs = []corev1.Service{*s, *dummySvc}
			},
			tester: func(t *testing.T, r *Renderer) {
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

				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err := r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testutils.TestStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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

				// config for the modified gateway-class
				gc = gcs[1]
				// we can never know the order...
				if gc.GetName() != "dummy-gateway-class" {
					gc = gcs[0]
				}

				assert.Equal(t, "dummy-gateway-class", gc.GetName(),
					"gatewayclass name")

				c = &RenderContext{gc: gc, log: logr.Discard()}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err = r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms = c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o = cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), "dummy-stunner-config",
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok = o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err = store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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
				assert.Equal(t, lc.Routes[0], "testnamespace/dummy-route", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc = conf.Clusters[0]
				assert.Equal(t, "testnamespace/dummy-route", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "dummy-service.testnamespace.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore EDS
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
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
			prep: func(c *renderTestConfig) {
				// a new gatewayclass that specifies a different gateway-config
				// a new gatewayclass that specifies a different gateway-config
				dummyGc := testutils.TestGwClass.DeepCopy()
				dummyGc.SetName("dummy-gateway-class")
				dummyGc.Spec.ParametersRef = &gwapiv1a2.ParametersReference{
					Group:     gwapiv1a2.Group(stnrv1a1.GroupVersion.Group),
					Kind:      gwapiv1a2.Kind("GatewayConfig"),
					Name:      "dummy-gateway-config",
					Namespace: &testutils.TestNs,
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
				dummyUdp := testutils.TestUDPRoute.DeepCopy()
				dummyUdp.SetName("dummy-route")
				dummyUdp.Spec.CommonRouteSpec.ParentRefs[0].Name = "dummy-gateway"
				dummyUdp.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gwapiv1a2.ObjectName("dummy-service")
				c.rs = []gwapiv1a2.UDPRoute{*dummyUdp, testutils.TestUDPRoute}

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

				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err := r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testutils.TestStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// config for the modified gateway-class
				gc = gcs[1]
				// we can never know the order...
				if gc.GetName() != "dummy-gateway-class" {
					gc = gcs[0]
				}

				assert.Equal(t, "dummy-gateway-class", gc.GetName(),
					"gatewayclass name")

				c = &RenderContext{gc: gc, log: logr.Discard()}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err = r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms = c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o = cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), "dummy-stunner-config",
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok = o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err = store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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
				assert.Equal(t, lc.Routes[0], "testnamespace/dummy-route", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc = conf.Clusters[0]
				assert.Equal(t, "testnamespace/dummy-route", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-1")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore EDS
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "EDS with relay-to-cluster-IP - E2E rendering for multiple gateway-classes",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			rs:   []gwapiv1a2.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				// a new gatewayclass that specifies a different gateway-config
				dummyGc := testutils.TestGwClass.DeepCopy()
				dummyGc.SetName("dummy-gateway-class")
				dummyGc.Spec.ParametersRef = &gwapiv1a2.ParametersReference{
					Group:     gwapiv1a2.Group(stnrv1a1.GroupVersion.Group),
					Kind:      gwapiv1a2.Kind("GatewayConfig"),
					Name:      "dummy-gateway-config",
					Namespace: &testutils.TestNs,
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
				dummyUdp := testutils.TestUDPRoute.DeepCopy()
				dummyUdp.SetName("dummy-route")
				dummyUdp.Spec.CommonRouteSpec.ParentRefs[0].Name = "dummy-gateway"
				dummyUdp.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gwapiv1a2.ObjectName("dummy-service")
				c.rs = []gwapiv1a2.UDPRoute{*dummyUdp, testutils.TestUDPRoute}

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
				config.EnableEndpointDiscovery = true
				config.EnableRelayToClusterIP = true

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

				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err := r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testutils.TestStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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

				// config for the modified gateway-class
				gc = gcs[1]
				// we can never know the order...
				if gc.GetName() != "dummy-gateway-class" {
					gc = gcs[0]
				}

				assert.Equal(t, "dummy-gateway-class", gc.GetName(),
					"gatewayclass name")

				c = &RenderContext{gc: gc, log: logr.Discard()}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				err = r.renderGatewayClass(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms = c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o = cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), "dummy-stunner-config",
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok = o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				conf, err = store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
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
				assert.Equal(t, lc.Routes[0], "testnamespace/dummy-route", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc = conf.Clusters[0]
				assert.Equal(t, "testnamespace/dummy-route", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-1")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore EDS
				config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
				config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
			},
		},
	})
}
