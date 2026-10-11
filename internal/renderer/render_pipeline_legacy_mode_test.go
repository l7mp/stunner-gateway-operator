package renderer

import (
	// "context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func TestRenderPipelineLegacyMode(t *testing.T) {
	// legacy mode
	renderTester(t, []renderTestConfig{
		{
			name: "piecewise render",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				admin, err := r.renderAdmin(c)
				assert.NoError(t, err, "admin rendered")
				assert.Equal(t, "testloglevel", admin.LogLevel, "log level")
				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					admin.Name, "stunnerd name")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "auth rendered")
				assert.Equal(t, stnrapiv2.AuthTypeStatic.String(),
					auth.Type, "auth type")
				assert.Equal(t, "testrealm", auth.Realm, "realm")
				assert.Equal(t, "testuser", auth.Credentials["username"], "username")
				assert.Equal(t, "testpass", auth.Credentials["password"], "password")

				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		{
			name: "no relay-to-cluster-IP - E2E test",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				// update owner ref so that we accept the public IP
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy

				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				c.gws.ResetGateways(r.getGateways4Class(c))
				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), opdefault.DefaultConfigMapName,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")

				cm := asConfigMap(o)
				assert.NotNil(t, cm, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auths[0].Realm, "realm")
				assert.Equal(t, "static", conf.Auths[0].Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auths[0].Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auths[0].Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(&conf, lc).Clusters[0], "testnamespace/udproute-ok/turn/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/turn/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Equal(t, []string{"$STUNNER_ADDR"}, rc.Addrs, "the relay address, resolved in the pod")
				assert.Len(t, rc.Endpoints, 4, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Empty(t, rc.Filters, "no port filter")

				// the listener binds every address, its server is named after it
				lc = conf.Listeners[0]
				assert.Equal(t, "", lc.Addr, "bind address")
				assert.Equal(t, []string{lc.Name}, lc.Servers, "servers")
				sc := serverOf(&conf, lc)
				assert.Equal(t, "default", sc.TURN.Auth, "auth policy")
				assert.Equal(t, "default", conf.Auths[0].Name, "auth policy name")

				// a permission filter per cluster, admitting its peers; a server without clusters
				// has none, denying every peer
				udpSrv := "testnamespace/gateway-1/gateway-1-listener-udp"
				assert.Equal(t, []string{udpSrv + "/testnamespace/udproute-ok/turn/udp"},
					sc.TURN.PermissionFilters, "permission filters")
				assert.Empty(t, serverOf(&conf, conf.Listeners[1]).TURN.PermissionFilters, "permission filters")

				assert.Len(t, conf.Filters, 1, "filter num")
				f, err := conf.GetFilterConfig(udpSrv + "/testnamespace/udproute-ok/turn/udp")
				assert.NoError(t, err, "permission filter found")
				assert.Len(t, f.Rules, 4, "permission filter rules")
				for _, r := range f.Rules {
					assert.Equal(t, "ALLOW", r.Action, "permission filter action")
					assert.Empty(t, r.Ports, "permission filter ports")
				}

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		{
			name:  "port range and multiple routes - E2E test",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:  []corev1.Service{testutils.TestSvc},
			esls:  []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			ssvcs: []stnrgwv1.StaticService{testutils.TestStaticSvc},
			prep: func(c *renderTestConfig) {
				port, endPort := gwapiv1.PortNumber(100), gwapiv1.PortNumber(200)
				ro1 := testutils.TestUDPRoute.DeepCopy()
				ro1.Spec.Rules[0].BackendRefs[0].Port = &port
				ro1.Spec.Rules[0].BackendRefs[0].EndPort = &endPort

				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				ro2 := testutils.TestUDPRoute.DeepCopy()
				ro2.SetName("route-2")
				ro2.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*ro1, *ro2}
			},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy
				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))
				assert.NoError(t, r.renderForGateways(c), "render success")

				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				conf, err := store.UnpackConfigMap(asConfigMap(cms[0]))
				assert.NoError(t, err, "configmap stunner-config unmarshal")

				assert.Len(t, conf.Clusters, 2, "cluster num")
				c1, err := conf.GetClusterConfig("testnamespace/udproute-ok/turn/udp")
				assert.NoError(t, err, "cluster 1 found")
				assert.Equal(t, []string{"testnamespace/udproute-ok/turn/udp"}, c1.Filters, "port filter")
				c2, err := conf.GetClusterConfig("testnamespace/route-2/turn/udp")
				assert.NoError(t, err, "cluster 2 found")
				assert.Empty(t, c2.Filters, "no port filter")
				assert.Equal(t, []string{"10.11.12.13", "10.11.12.14", "10.11.12.15"}, c2.Endpoints, "endpoints")

				// the port filter of cluster 1: its peers on the backend ports, then a deny
				f, err := conf.GetFilterConfig("testnamespace/udproute-ok/turn/udp")
				assert.NoError(t, err, "port filter found")
				assert.Len(t, f.Rules, 5, "port filter rules")
				for _, r := range f.Rules[:4] {
					assert.Equal(t, "ALLOW", r.Action, "port filter action")
					assert.Equal(t, []stnrapiv2.FilterPort{{Port: 100, EndPort: 200}}, r.Ports, "port filter ports")
				}
				assert.Equal(t, "DENY", f.Rules[4].Action, "port filter deny")

				// the permission filters of the UDP listener: per cluster in order
				lc, err := conf.GetListenerConfig("testnamespace/gateway-1/gateway-1-listener-udp")
				assert.NoError(t, err, "listener found")
				sc := serverOf(&conf, lc)
				assert.Len(t, sc.Clusters, 2, "route num")
				pfs := []string{}
				for _, cn := range sc.Clusters {
					pfs = append(pfs, sc.Name+"/"+cn)
				}
				assert.Equal(t, pfs, sc.TURN.PermissionFilters, "permission filters")

				f, err = conf.GetFilterConfig(sc.Name + "/testnamespace/udproute-ok/turn/udp")
				assert.NoError(t, err, "permission filter 1 found")
				assert.Len(t, f.Rules, 4, "permission filter 1 rules")
				for _, r := range f.Rules {
					assert.Equal(t, "ALLOW", r.Action, "permission filter action")
					assert.Empty(t, r.Ports, "permission filter ports")
				}
				f, err = conf.GetFilterConfig(sc.Name + "/testnamespace/route-2/turn/udp")
				assert.NoError(t, err, "permission filter 2 found")
				assert.Len(t, f.Rules, 3, "permission filter 2 rules")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		{
			name: "relay-to-cluster-IP - E2E test",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				// update owner ref so that we accept the public IP
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy

				config.EnableRelayToClusterIP = true

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				c.gws.ResetGateways(r.getGateways4Class(c))
				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), opdefault.DefaultConfigMapName,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")

				cm := asConfigMap(o)
				assert.NotNil(t, cm, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auths[0].Realm, "realm")
				assert.Equal(t, "static", conf.Auths[0].Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auths[0].Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auths[0].Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(&conf, lc).Clusters[0], "testnamespace/udproute-ok/turn/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/turn/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 5, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3")
				assert.Contains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4")
				assert.Contains(t, rc.Endpoints, "4.3.2.1", "cluster-ip")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		{
			name: "E2E invalidation",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
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
				assert.Equal(t, o.GetName(), opdefault.DefaultConfigMapName,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")

				cm := asConfigMap(o)
				assert.NotNil(t, cm, "configmap cast")

				conf, found := cm.Data[opdefault.DefaultStunnerdConfigfileName]
				assert.True(t, found, "configmap data: stunnerd.conf found")
				assert.Equal(t, "", conf, "configmap data: stunnerd.conf empty")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				//statuses
				setGatewayClassStatusAccepted(gc, nil)
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				assert.Equal(t, string(gwapiv1.GatewayClassConditionStatusAccepted),
					gc.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, metav1.ConditionTrue,
					gc.Status.Conditions[0].Status, "conditions status")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonAccepted),
					gc.Status.Conditions[0].Type, "conditions reason")
				assert.Equal(t, int64(0),
					gc.Status.Conditions[0].ObservedGeneration, "conditions gen")

				gws := c.update.UpsertQueue.Gateways.Objects()
				assert.Len(t, gws, 1, "gateway num")
				gw, ok := gws[0].(*gwapiv1.Gateway)
				assert.True(t, ok, "gateway cast")
				assert.NotNil(t, gw, "gateway found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

				assert.Len(t, gw.Status.Conditions, 2, "conditions num")

				assert.Equal(t, string(gwapiv1.GatewayConditionAccepted),
					gw.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, int64(0), gw.Status.Conditions[0].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionFalse, gw.Status.Conditions[0].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonPending),
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
			name: "relay-to-cluster-IP - E2E rendering for multiple gateway-classes",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				config.EnableRelayToClusterIP = false

				// a new gatewayclass that specifies a different gateway-config
				dummyGc := testutils.TestGwClass.DeepCopy()
				dummyGc.SetName("dummy-gateway-class")
				dummyNs := gwapiv1.Namespace("dummy-namespace")
				dummyGc.Spec.ParametersRef = &gwapiv1.ParametersReference{
					Group:     gwapiv1.Group(stnrgwv1.GroupVersion.Group),
					Kind:      gwapiv1.Kind("GatewayConfig"),
					Name:      "dummy-gateway-config",
					Namespace: &dummyNs,
				}
				c.cls = []gwapiv1.GatewayClass{testutils.TestGwClass, *dummyGc}

				// the new gateway-config that renders into a different stunner configmap
				dummyConf := testutils.TestGwConfig.DeepCopy()
				dummyConf.SetName("dummy-gateway-config")
				dummyConf.SetNamespace(string(dummyNs))
				c.cfs = []stnrgwv1.GatewayConfig{testutils.TestGwConfig, *dummyConf}

				// a new gateway whose controller-name is the new gatewayclass
				dummyGw := testutils.TestGw.DeepCopy()
				dummyGw.SetName("dummy-gateway")
				dummyGw.SetNamespace(string(dummyNs))
				dummyGw.Spec.GatewayClassName = gwapiv1.ObjectName("dummy-gateway-class")
				c.gws = []gwapiv1.Gateway{*dummyGw, testutils.TestGw}

				// a route for dummy-gateway
				dummyUdp := testutils.TestUDPRoute.DeepCopy()
				dummyUdp.SetName("dummy-route")
				dummyUdp.SetNamespace(string(dummyNs))
				dummyUdp.Spec.CommonRouteSpec.ParentRefs[0].Name = "dummy-gateway"
				dummyUdp.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gwapiv1.ObjectName("dummy-service")
				dummyUdp.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Namespace =
					&testutils.TestNsName
				c.rs = []stnrgwv1.UDPRoute{*dummyUdp, testutils.TestUDPRoute}

				s := testutils.TestSvc.DeepCopy()
				s.Spec.ClusterIP = "4.3.2.1"
				// update owner ref so that we accept the public IP
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				dummySvc := testutils.TestSvc.DeepCopy()
				dummySvc.SetName("dummy-service")
				c.svcs = []corev1.Service{*s, *dummySvc}

				dummyEp := testutils.TestEndpointSlice.DeepCopy()
				dummyEp.SetName("dummy-service-endpointslice")
				dummyEp.SetLabels(map[string]string{"kubernetes.io/service-name": "dummy-service"})
				dummyEp.Endpoints = []discoveryv1.Endpoint{{
					Addresses: []string{"4.4.4.4"},
					Conditions: discoveryv1.EndpointConditions{
						Ready:   &testutils.TestTrue,
						Serving: &testutils.TestTrue,
					},
				}}
				c.esls = []discoveryv1.EndpointSlice{testutils.TestEndpointSlice, *dummyEp}
			},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy

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

				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gwConf, err := r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gateway-conf obtained")
				c.gwConf = gwConf
				c.gws.ResetGateways(r.getGateways4Class(c))

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), opdefault.DefaultConfigMapName,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")

				cm := asConfigMap(o)
				assert.NotNil(t, cm, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auths[0].Realm, "realm")
				assert.Equal(t, "static", conf.Auths[0].Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auths[0].Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auths[0].Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(&conf, lc).Clusters[0], "testnamespace/udproute-ok/turn/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/turn/udp", rc.Name, "cluster name")
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

				c = &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gateway-conf obtained")
				c.gws.ResetGateways(r.getGateways4Class(c))

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms = c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")

				o = cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), opdefault.DefaultConfigMapName, "configmap name")
				assert.Equal(t, o.GetNamespace(), "dummy-namespace", "configmap namespace")

				// related gw
				as = o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok = as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")

				cm = asConfigMap(o)
				assert.NotNil(t, cm, "configmap cast")

				conf, err = store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarschal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auths[0].Realm, "realm")
				assert.Equal(t, "static", conf.Auths[0].Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auths[0].Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auths[0].Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc = conf.Listeners[0]
				assert.Equal(t, "dummy-namespace/dummy-gateway/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				// the service links to the original gateway, our gateway does not
				// have linkage, so public addr should be empty
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(&conf, lc).Clusters[0], "dummy-namespace/dummy-route/turn/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "dummy-namespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc = conf.Clusters[0]
				assert.Equal(t, "dummy-namespace/dummy-route/turn/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-1")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		{
			name:  "StaticService - E2E test",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			svcs:  []corev1.Service{testutils.TestSvc},
			ssvcs: []stnrgwv1.StaticService{testutils.TestStaticSvc},
			prep: func(c *renderTestConfig) {
				// update owner ref so that we accept the public IP
				s := testutils.TestSvc.DeepCopy()
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: stnrgwv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}

				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				udp := testutils.TestUDPRoute.DeepCopy()
				udp.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udp}
			},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gateway-conf obtained")
				c.gws.ResetGateways(r.getGateways4Class(c))

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), opdefault.DefaultConfigMapName,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")

				cm := asConfigMap(o)
				assert.NotNil(t, cm, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarshal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auths[0].Realm, "realm")
				assert.Equal(t, "static", conf.Auths[0].Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auths[0].Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auths[0].Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(&conf, lc).Clusters[0], "testnamespace/udproute-ok/turn/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/turn/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 3, "endpoints len")
				assert.Contains(t, rc.Endpoints, "10.11.12.13", "staticservice endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "10.11.12.14", "staticservice endpoint ip-2")
				assert.Contains(t, rc.Endpoints, "10.11.12.15", "staticservice endpoint ip-3")

				//statuses
				setGatewayClassStatusAccepted(gc, nil)
				assert.Len(t, gc.Status.Conditions, 1, "conditions num")
				assert.Equal(t, string(gwapiv1.GatewayClassConditionStatusAccepted),
					gc.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, metav1.ConditionTrue,
					gc.Status.Conditions[0].Status, "conditions status")
				assert.Equal(t, string(gwapiv1.GatewayClassReasonAccepted),
					gc.Status.Conditions[0].Type, "conditions reason")
				assert.Equal(t, int64(0),
					gc.Status.Conditions[0].ObservedGeneration, "conditions gen")

				gws := c.update.UpsertQueue.Gateways.Objects()
				assert.Len(t, gws, 1, "gateway num")
				gw, ok := gws[0].(*gwapiv1.Gateway)
				assert.True(t, ok, "gateway cast")
				assert.NotNil(t, gw, "gateway found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gw), "gw name found")

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
				assert.Equal(t, metav1.ConditionTrue, gw.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonProgrammed),
					gw.Status.Conditions[1].Reason, "reason")

				ros := c.update.UpsertQueue.UDPRoutes.Objects()
				assert.Len(t, ros, 1, "routenum")
				ro, ok := ros[0].(*stnrgwv1.UDPRoute)
				assert.True(t, ok, "udproute cast")
				assert.NotNil(t, ro, "route found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "udproute-ok"),
					store.GetObjectKey(ro), "route name found")

				assert.Len(t, ro.GetRouteStatus().Parents, 1, "parent status len")
				p := ro.GetParentRefs()[0]
				parentStatus := ro.GetRouteStatus().Parents[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d := meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1.RouteConditionAccepted))
				assert.NotNil(t, d, "accepted found")
				assert.Equal(t, string(gwapiv1.RouteConditionAccepted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "Accepted", d.Reason, "reason")

				d = meta.FindStatusCondition(parentStatus.Conditions,
					string(gwapiv1.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, string(gwapiv1.RouteConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, "ResolvedRefs", d.Reason, "reason")

				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
		// {
		// 	name: "reject uncontrolled route",
		// 	cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
		// 	cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
		// 	gws:  []gwapiv1.Gateway{testutils.TestGw},
		// 	rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
		// 	prep: func(c *renderTestConfig) {
		// 		// a new gatewayclass that specifies a different gateway-config
		// 		dummyGc := testutils.TestGwClass.DeepCopy()
		// 		dummyGc.SetName("dummy-gateway-class")
		// 		dummyGc.Spec.ControllerName = "dummy-controller"
		// 		dummyGc.Spec.ParametersRef = &gwapiv1.ParametersReference{
		// 			Group:     gwapiv1.Group(stnrgwv1.GroupVersion.Group),
		// 			Kind:      gwapiv1.Kind("GatewayConfig"),
		// 			Name:      "dummy-gateway-config",
		// 			Namespace: &testutils.TestNsName,
		// 		}
		// 		c.cls = []gwapiv1.GatewayClass{testutils.TestGwClass, *dummyGc}

		// 		// the new gateway-config that renders into a different stunner configmap
		// 		dummyConf := testutils.TestGwConfig.DeepCopy()
		// 		dummyConf.SetName("dummy-gateway-config")
		// 		target := "dummy-stunner-config"
		// 		dummyConf.Spec.StunnerConfig = &target
		// 		c.cfs = []stnrgwv1.GatewayConfig{testutils.TestGwConfig, *dummyConf}

		// 		// a new gateway whose controller-name is the new gatewayclass
		// 		dummyGw := testutils.TestGw.DeepCopy()
		// 		dummyGw.SetName("dummy-gateway")
		// 		dummyGw.Spec.GatewayClassName =
		// 			gwapiv1.ObjectName("dummy-gateway-class")
		// 		c.gws = []gwapiv1.Gateway{*dummyGw, testutils.TestGw}

		// 		// a route for dummy-gateway
		// 		dummyUdp := testutils.TestUDPRoute.DeepCopy()
		// 		dummyUdp.SetName("dummy-route")
		// 		dummyUdp.Spec.CommonRouteSpec.ParentRefs[0].Name = "dummy-gateway"
		// 		dummyUdp.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
		// 			gwapiv1.ObjectName("dummy-service")
		// 		c.rs = []stnrgwv1.UDPRoute{*dummyUdp}
		// 	},
		// 	tester: func(t *testing.T, r *DefaultRenderer) {
		// 		config.DataplaneMode = config.DataplaneModeLegacy

		// 		gcs := r.getGatewayClasses()
		// 		assert.Len(t, gcs, 1, "gw-classes found")

		// 		// render our own gatewayclass
		// 		c := &RenderContext{gc: gcs[0], gws: store.NewGatewayStore(), log: log}
		// 		c.update = event.NewEventUpdate(0)
		// 		assert.NotNil(t, c.update, "update event create")

		// 		gwConf, err := r.getGatewayConfig4Class(c)
		// 		assert.NoError(t, err, "gateway-conf obtained")
		// 		c.gwConf = gwConf
		// 		c.gws.ResetGateways(r.getGateways4Class(c))

		// 		err = r.renderForGateways(c)
		// 		assert.NoError(t, err, "render success")

		// 		// the update list should contain zero routes
		// 		ros := c.update.UpsertQueue.UDPRoutes.Objects()
		// 		assert.Len(t, ros, 0, "routenum")

		// 		// after the render our udpRoute should have an empty status
		// 		// (handled by another controller)
		// 		udpRoutes := store.UDPRoutes.GetAll()
		// 		assert.Len(t, udpRoutes, 1, "routenum")
		// 		ro := udpRoutes[0]
		// 		assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "dummy-route"),
		// 			store.GetObjectKey(ro), "route name found")

		// 		assert.Len(t, ro.GetRouteStatus().Parents, 0, "parent status len")

		// 		config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
		// 	},
		// },
		{
			name: "Address hint set in Gw.spec.addresses",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// update owner ref so that we accept the public IP
				s := testutils.TestSvc.DeepCopy()
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}

				gw := testutils.TestGw.DeepCopy()
				at := gwapiv1.IPAddressType
				gw.Spec.Addresses = []gwapiv1.GatewaySpecAddress{
					{
						Type:  &at,
						Value: "1.1.1.1",
					},
				}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				config.DataplaneMode = config.DataplaneModeLegacy

				gcs := r.getGatewayClasses()
				assert.Len(t, gcs, 1, "gw-classes found")

				// render our own gatewayclass
				c := &RenderContext{gc: gcs[0], gws: store.NewGatewayStore(), log: log}
				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gwConf, err := r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gateway-conf obtained")
				c.gwConf = gwConf
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")
				c.gws.ResetGateways(r.getGateways4Class(c))

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// configmap
				cms := c.update.UpsertQueue.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), opdefault.DefaultConfigMapName,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				// related gw
				as := o.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				_, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")

				cm := asConfigMap(o)
				assert.NotNil(t, cm, "configmap cast")

				conf, err := store.UnpackConfigMap(cm)
				assert.NoError(t, err, "configmap stunner-config unmarshal")

				assert.Equal(t, opdefault.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auths[0].Realm, "realm")
				assert.Equal(t, "static", conf.Auths[0].Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auths[0].Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auths[0].Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 2, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.1.1.1", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(&conf, lc).Clusters[0], "testnamespace/udproute-ok/turn/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(&conf, lc).Type, "server type")
				assert.Equal(t, "1.1.1.1", lc.PublicAddr, "public-ip")
				// assert.Equal(t, int(testutils.TestMinPort), lc.MinRelayPort, "min-port")
				// assert.Equal(t, int(testutils.TestMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, serverOf(&conf, lc).Clusters, 0, "route num")

				config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
			},
		},
	})
}
