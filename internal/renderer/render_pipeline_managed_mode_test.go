package renderer

import (
	"errors"
	"fmt"
	"testing"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderPipelineManagedMode(t *testing.T) {
	// managed mode
	renderTester(t, []renderTestConfig{
		{
			name: "relay-to-cluster-IP - E2E test",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
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

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 1, lc.Port, "port")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 2, lc.Port, "port")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 2, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 2, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4: not ready, not dialed")
				assert.Contains(t, serverFilterBlocks(conf, rc.Name), "4.3.2.1/32", "cluster-ip: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name:    "Gateway API UDPRoute with relay-to-cluster-IP - E2E test",
			cls:     []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:     []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:     []gwapiv1.Gateway{testutils.TestGw},
			rsGwAPI: []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs:    []corev1.Service{testutils.TestSvc},
			esls:    []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:     []stnrgwv1.Dataplane{testutils.TestDataplane},
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

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 1, lc.Port, "port")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 2, lc.Port, "port")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 2, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 2, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4: not ready, not dialed")
				assert.Contains(t, serverFilterBlocks(conf, rc.Name), "4.3.2.1/32", "cluster-ip: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "multi-listener GW with relay-to-cluster-IP - E2E test",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec = gwapiv1.GatewaySpec{
					GatewayClassName: "gatewayclass-ok",
					Listeners: []gwapiv1.Listener{{
						Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
						Port:     gwapiv1.PortNumber(1),
						Protocol: gwapiv1.ProtocolType("TURN-UDP"),
					}, {
						Name:     gwapiv1.SectionName("gateway-1-listener-tcp"),
						Port:     gwapiv1.PortNumber(2),
						Protocol: gwapiv1.ProtocolType("TURN-TCP"),
					}},
				}
				c.gws = []gwapiv1.Gateway{*gw}

				s1 := testutils.TestSvc.DeepCopy()
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				s1.Spec.ClusterIP = "4.3.2.1"
				s1.Spec.Ports = []corev1.ServicePort{
					{
						Name:     "udp-ok",
						Protocol: corev1.ProtocolUDP,
						Port:     1,
					},
					{
						Name:     "tcp-ok",
						Protocol: corev1.ProtocolTCP,
						Port:     2,
					},
				}
				s1.Status = corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{
							IP: "1.2.3.4",
							Ports: []corev1.PortStatus{{
								Port:     1,
								Protocol: corev1.ProtocolUDP,
							}, {
								Port:     2,
								Protocol: corev1.ProtocolTCP,
							}},
						}},
					}}
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
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

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 1, lc.Port, "port")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 2, lc.Port, "port")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 2, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 2, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4: not ready, not dialed")
				assert.Contains(t, serverFilterBlocks(conf, rc.Name), "4.3.2.1/32", "cluster-ip: a peer, no endpoint")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "E2E invalidation",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				r.invalidateGatewayClass(c, errors.New("dummy"))

				// we do not render a zero config on invalidation, but rather
				// remove the deployment all together
				objs := c.update.DeleteQueue.Deployments.Objects()
				assert.Len(t, objs, 1, "deployment num")
				o := objs[0]
				assert.Equal(t, o.GetName(), gw.GetName(), "deployment name")
				assert.Equal(t, o.GetNamespace(), gw.GetNamespace(), "deployment namespace")

				//statuses
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

				objs = c.update.UpsertQueue.Gateways.Objects()
				assert.Len(t, gws, 1, "gateway num")
				gwStatus, ok := objs[0].(*gwapiv1.Gateway)
				assert.True(t, ok, "gateway cast")
				assert.NotNil(t, gwStatus, "gateway found")
				assert.Equal(t, fmt.Sprintf("%s/%s", testutils.TestNsName, "gateway-1"),
					store.GetObjectKey(gwStatus), "gw name found")

				assert.Len(t, gwStatus.Status.Conditions, 2, "conditions num")

				assert.Equal(t, string(gwapiv1.GatewayConditionAccepted),
					gwStatus.Status.Conditions[0].Type, "conditions accepted")
				assert.Equal(t, int64(0), gwStatus.Status.Conditions[0].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionFalse, gwStatus.Status.Conditions[0].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonPending),
					gwStatus.Status.Conditions[0].Reason, "reason")

				assert.Equal(t, string(gwapiv1.GatewayConditionProgrammed),
					gwStatus.Status.Conditions[1].Type, "programmed")
				assert.Equal(t, int64(0), gwStatus.Status.Conditions[1].ObservedGeneration,
					"conditions gen")
				assert.Equal(t, metav1.ConditionFalse, gwStatus.Status.Conditions[1].Status,
					"status")
				assert.Equal(t, string(gwapiv1.GatewayReasonInvalid),
					gwStatus.Status.Conditions[1].Reason, "reason")
			},
		},
		{
			name: "no relay-to-cluster-IP - E2E rendering for multiple gateway-classes",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				// a new gatewayclass that specifies a different gateway-config
				dummyGc := testutils.TestGwClass.DeepCopy()
				dummyGc.SetName("dummy-gateway-class")
				dummyGc.Spec.ParametersRef = &gwapiv1.ParametersReference{
					Group:     gwapiv1.Group(stnrgwv1.GroupVersion.Group),
					Kind:      gwapiv1.Kind("GatewayConfig"),
					Name:      "dummy-gateway-config",
					Namespace: &testutils.TestNsName,
				}
				c.cls = []gwapiv1.GatewayClass{testutils.TestGwClass, *dummyGc}

				// the new gateway-config that renders into a different stunner config
				dummyConf := testutils.TestGwConfig.DeepCopy()
				dummyConf.SetName("dummy-gateway-config")
				c.cfs = []stnrgwv1.GatewayConfig{testutils.TestGwConfig, *dummyConf}

				// a new gateway whose controller-name is the new gatewayclass
				dummyGw := testutils.TestGw.DeepCopy()
				dummyGw.SetName("dummy-gateway")
				dummyGw.Spec.GatewayClassName =
					gwapiv1.ObjectName("dummy-gateway-class")
				c.gws = []gwapiv1.Gateway{*dummyGw, testutils.TestGw}

				// a route for dummy-gateway
				sn := gwapiv1.SectionName("gateway-1-listener-udp")
				udpRoute := testutils.TestUDPRoute.DeepCopy()
				udpRoute.Spec.CommonRouteSpec.ParentRefs = []gwapiv1.ParentReference{{
					Name:      "dummy-gateway",
					Namespace: &testutils.TestNsName,
				}, {
					Name:        gwapiv1.ObjectName(testutils.TestGw.GetName()),
					Namespace:   &testutils.TestNsName,
					SectionName: &sn,
				}}
				port := gwapiv1.PortNumber(10)
				endPort := gwapiv1.PortNumber(100)
				udpRoute.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name:      gwapiv1.ObjectName(testutils.TestSvc.GetName()),
						Namespace: &testutils.TestNsName,
						Port:      &port,
						EndPort:   &endPort,
					},
				}, {
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name:      "dummy-service",
						Namespace: &testutils.TestNsName,
					},
				}}
				c.rs = []stnrgwv1.UDPRoute{*udpRoute}

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
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
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
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 3, "endpoints len")
				// at the backend port
				assert.Contains(t, rc.Endpoints, "1.2.3.4:10", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5:10", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "endpoint ip-4: not ready, not dialed")
				assert.Empty(t, rc.Filters, "a cluster has no filter")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-5") // this one comes from another backendRef

				// gateway status
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

				assert.Len(t, gw.Status.Listeners, 2, "conditions num")

				// listeners[0]: ok
				assert.Equal(t, "gateway-1-listener-udp", string(gw.Status.Listeners[0].Name), "name")
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
				assert.Equal(t, "gateway-1-listener-tcp", string(gw.Status.Listeners[1].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted), d.Reason,
					"reason")

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

				// route status
				ros := r.allRoutes()
				assert.Len(t, ros, 1, "routes len")
				ro := ros[0]

				assert.Len(t, ro.GetRouteStatus().Parents, 2, "parent status len")
				parentStatus := ro.GetRouteStatus().Parents[0]
				p := ro.GetParentRefs()[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
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

				parentStatus = ro.GetRouteStatus().Parents[1]
				p = ro.GetParentRefs()[1]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
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

				// config for the modified gateway-class
				gc = gcs[1]
				// we can never know the order...
				if gc.GetName() != "dummy-gateway-class" {
					gc = gcs[0]
				}
				assert.Equal(t, "dummy-gateway-class", gc.GetName(),
					"gatewayclass name")

				c = &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
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
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// config-queue
				cs = c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf = cs[0]

				assert.Equal(t, "testnamespace/dummy-gateway", conf.Admin.Name, "name")
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
				assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				// the service links to the original gateway, our gateway does not
				// have linkage, so public addr should be empty
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc = conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 3, "endpoints len")
				// at the backend port
				assert.Contains(t, rc.Endpoints, "1.2.3.4:10", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5:10", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "endpoint ip-4: not ready, not dialed")
				assert.Empty(t, rc.Filters, "a cluster has no filter")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-5")

				// gateway status
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
				assert.Equal(t, string(gwapiv1.GatewayReasonAddressNotAssigned),
					gw.Status.Conditions[1].Reason, "reason")

				// route status
				assert.Len(t, ro.GetRouteStatus().Parents, 2, "parent status len")
				parentStatus = ro.GetRouteStatus().Parents[0]
				p = ro.GetParentRefs()[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
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

				parentStatus = ro.GetRouteStatus().Parents[1]
				p = ro.GetParentRefs()[1]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
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

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "no relay-to-cluster-IP - E2E test - conflicted status",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("udp-ok"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("udp-conflicted"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("dtls-conflicted"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"),
				}, {
					Name:     gwapiv1.SectionName("tcp"),
					Port:     gwapiv1.PortNumber(11),
					Protocol: gwapiv1.ProtocolType("TURN-TCP"),
				}, {
					Name:     gwapiv1.SectionName("tcp-ok"),
					Port:     gwapiv1.PortNumber(12),
					Protocol: gwapiv1.ProtocolType("TURN-TCP"),
				}, {
					Name:     gwapiv1.SectionName("tcp-conflicted"),
					Port:     gwapiv1.PortNumber(11),
					Protocol: gwapiv1.ProtocolType("TURN-TCP"),
				}, {
					Name:     gwapiv1.SectionName("tls-conflicted"),
					Port:     gwapiv1.PortNumber(11),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				// attach to all listeners
				ro := testutils.TestUDPRoute.DeepCopy()
				ro.Spec.CommonRouteSpec.ParentRefs = []gwapiv1.ParentReference{{Name: "gateway-1"}}
				c.rs = []stnrgwv1.UDPRoute{*ro}

				// update owner ref so that we accept the public IP
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

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
				assert.Equal(t, testutils.TestLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testutils.TestRealm, conf.Auths[0].Realm, "realm")
				assert.Equal(t, "static", conf.Auths[0].Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, conf.Auths[0].Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, conf.Auths[0].Credentials["password"],
					"password")

				assert.Len(t, conf.Listeners, 4, "listener num")
				lc := conf.Listeners[0]
				assert.Equal(t, "testnamespace/gateway-1/udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 1, lc.Port, "port")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/udp-ok", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 2, lc.Port, "port")
				assert.Equal(t, "", lc.PublicAddr, "public-ip") // no service-port for udp:2
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[2]
				assert.Equal(t, "testnamespace/gateway-1/tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 11, lc.Port, "port")
				assert.Equal(t, "", lc.PublicAddr, "public-ip") // no service-port for tcp:11
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[3]
				assert.Equal(t, "testnamespace/gateway-1/tcp-ok", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 12, lc.Port, "port")
				assert.Equal(t, "", lc.PublicAddr, "public-ip") // no service-port for tcp:11
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 2, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4: not ready, not dialed")

				// deployment
				dps := c.update.UpsertQueue.Deployments.Objects()
				assert.Len(t, dps, 1, "deployment num")
				deploy := asDeployment(dps[0])
				assert.NotNil(t, deploy, "deployment cast")

				assert.Equal(t, gw.GetName(), deploy.GetName(), "deployment name")
				assert.Equal(t, gw.GetNamespace(), deploy.GetNamespace(), "deployment namespace")

				labs := deploy.GetLabels()
				assert.Len(t, labs, 4, "labels len")
				v, ok := labs[opdefault.OwnedByLabelKey]
				assert.True(t, ok, "labels: app")
				assert.Equal(t, opdefault.OwnedByLabelValue, v, "app label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw label value")
				// label from the dataplane object
				v, ok = labs["dummy-label"]
				assert.True(t, ok, "labels: dataplane label copied")
				assert.Equal(t, "dummy-value", v, "copied dataplane label value")

				as := deploy.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwName, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")
				// annotation is gw-namespace/gw-name
				assert.Equal(t, store.GetObjectKey(gw), gwName, "related-gateway annotation")

				// check the label selector
				labelSelector := deploy.Spec.Selector
				assert.NotNil(t, labelSelector, "label selector")

				selector, err := metav1.LabelSelectorAsSelector(labelSelector)
				assert.NoError(t, err, "label selector convert")

				// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
				// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
				labelToMatch := labels.Merge(
					labels.Merge(
						labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
						labels.Set{opdefault.RelatedGatewayKey: gw.GetName()},
					),
					labels.Set{opdefault.RelatedGatewayNamespace: gw.GetNamespace()},
				)
				assert.True(t, selector.Matches(labelToMatch), "selector matched")

				// spec
				assert.NotNil(t, deploy.Spec.Replicas, "replicas notnil")
				assert.Equal(t, int32(3), *deploy.Spec.Replicas, "replicas")
				assert.NotNil(t, deploy.Spec.Strategy, "strategy notnil")

				// pod template spec
				podTemplate := &deploy.Spec.Template
				labs = podTemplate.GetLabels()
				assert.Len(t, labs, 3, "labels len")
				v, ok = labs[opdefault.AppLabelKey]
				assert.True(t, ok, "labels: owned-by")
				assert.Equal(t, opdefault.AppLabelValue, v, "owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "labels: related namespace")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw-namespace label value")

				// deployment selector matches pod template
				assert.True(t, selector.Matches(labels.Set(labs)), "selector matched")

				podSpec := &podTemplate.Spec

				assert.Len(t, podSpec.Containers, 1, "contianers len")

				container := podSpec.Containers[0]
				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, container.Name, "container 1 name")
				assert.Equal(t, "testimage-1", container.Image, "container 1 image")
				assert.Equal(t, []string{"testcommand-1"}, container.Command, "container 1 command")
				assert.Equal(t, []string{"arg-1", "arg-2"}, container.Args, "container 1 args")

				assert.Equal(t, testutils.TestResourceLimit, container.Resources.Limits, "container 1 - resource limits")
				assert.Equal(t, testutils.TestResourceRequest, container.Resources.Requests, "container 1 - resource req")
				assert.Equal(t, corev1.PullAlways, container.ImagePullPolicy, "container 1 - readiness probe")

				// remainder
				assert.NotNil(t, podSpec.TerminationGracePeriodSeconds, "termination grace ptr")
				assert.Equal(t, testutils.TestTerminationGrace, *podSpec.TerminationGracePeriodSeconds, "termination grace")
				assert.True(t, podSpec.HostNetwork, "hostnetwork")
				assert.Nil(t, podSpec.Affinity, "affinity")

				// gateway status
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
					"status") // some public addresses are missing
				assert.Equal(t, string(gwapiv1.GatewayReasonAddressNotAssigned),
					gw.Status.Conditions[1].Reason, "reason")

				assert.Len(t, gw.Status.Listeners, 8, "conditions num")

				// listeners[0]: ok
				assert.Equal(t, "udp", string(gw.Status.Listeners[0].Name), "name")
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
				assert.Equal(t, "udp-ok", string(gw.Status.Listeners[1].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted), d.Reason,
					"reason")

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

				// listeners[2]: conflict
				assert.Equal(t, "udp-conflicted", string(gw.Status.Listeners[2].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonPortUnavailable), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonProtocolConflict),
					d.Reason, "reason")

				// listeners[3]: conflict
				assert.Equal(t, "dtls-conflicted", string(gw.Status.Listeners[3].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[3].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonPortUnavailable), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[3].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[3].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonProtocolConflict),
					d.Reason, "reason")

				// listeners[4]: ok
				assert.Equal(t, "tcp", string(gw.Status.Listeners[4].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[4].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[4].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[4].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonNoConflicts),
					d.Reason, "reason")

				// listeners[5]: ok
				assert.Equal(t, "tcp-ok", string(gw.Status.Listeners[5].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[5].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonAccepted), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[5].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[5].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonNoConflicts),
					d.Reason, "reason")

				// listeners[6]: conflict
				assert.Equal(t, "tcp-conflicted", string(gw.Status.Listeners[6].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[6].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonPortUnavailable), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[6].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[6].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonProtocolConflict),
					d.Reason, "reason")

				// listeners[7]: conflict
				assert.Equal(t, "tls-conflicted", string(gw.Status.Listeners[7].Name), "name")
				d = meta.FindStatusCondition(gw.Status.Listeners[7].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				assert.NotNil(t, d, "acceptedfound")
				assert.Equal(t, metav1.ConditionFalse, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonPortUnavailable), d.Reason,
					"reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[7].Conditions,
					string(gwapiv1.ListenerConditionResolvedRefs))
				assert.NotNil(t, d, "resovedrefs found")
				assert.Equal(t, string(gwapiv1.ListenerConditionResolvedRefs), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonResolvedRefs),
					d.Reason, "reason")

				d = meta.FindStatusCondition(gw.Status.Listeners[7].Conditions,
					string(gwapiv1.ListenerConditionConflicted))
				assert.NotNil(t, d, "conflicted found")
				assert.Equal(t, string(gwapiv1.ListenerConditionConflicted), d.Type,
					"type")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "status")
				assert.Equal(t, int64(0), d.ObservedGeneration, "gen")
				assert.Equal(t, string(gwapiv1.ListenerReasonProtocolConflict),
					d.Reason, "reason")

				// route status
				ros := r.allRoutes()
				assert.Len(t, ros, 1, "routes len")
				ro := ros[0]
				p := ro.GetParentRefs()[0]

				assert.Len(t, ro.GetRouteStatus().Parents, 1, "parent status len")
				parentStatus := ro.GetRouteStatus().Parents[0]

				assert.Equal(t, p.Group, parentStatus.ParentRef.Group, "status parent ref group")
				assert.Equal(t, p.Kind, parentStatus.ParentRef.Kind, "status parent ref kind")
				assert.Equal(t, p.Namespace, parentStatus.ParentRef.Namespace, "status parent ref namespace")
				assert.Equal(t, p.Name, parentStatus.ParentRef.Name, "status parent ref name")
				assert.Equal(t, p.SectionName, parentStatus.ParentRef.SectionName, "status parent ref section-name")

				assert.Equal(t, gwapiv1.GatewayController("stunner.l7mp.io/gateway-operator"),
					parentStatus.ControllerName, "status parent ref")

				d = meta.FindStatusCondition(parentStatus.Conditions,
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

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},

		{
			name: "dataplane disabled",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{
					opdefault.ManagedDataplaneDisabledAnnotationKey: opdefault.ManagedDataplaneDisabledAnnotationValue,
				})
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				dps := c.update.UpsertQueue.Deployments
				assert.Equal(t, 0, dps.Len(), "no deployment in upsert queue")

				dps = c.update.DeleteQueue.Deployments
				assert.Equal(t, 1, dps.Len(), "deployment found in delete queue")
			},
		},
		{
			name: "targetport enforced",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{
					opdefault.TargetPortAnnotationKey: "{\"gateway-1-listener-udp\":11,\"dummy\":12,\"gateway-1-listener-tcp\":13}",
				})
				c.gws = []gwapiv1.Gateway{*gw}

				// update owner ref so that we accept the public IP
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

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				err = r.renderForGateways(c)
				assert.NoError(t, err, "render success")

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 11, lc.Port, "port") // targetport!!!!
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 1, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, 13, lc.Port, "port") // targetport!!!
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, 2, lc.PublicPort, "public-port")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "piecewise render",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
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
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
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

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Equal(t, []string{"$STUNNER_ADDR"}, rc.Addrs, "the relay address, resolved in the pod")
				assert.Len(t, rc.Endpoints, 2, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4: not ready, not dialed")
				assert.Empty(t, rc.Filters, "no port filter")

				// the listener binds every address, its server is named after it
				lc = conf.Listeners[0]
				assert.Equal(t, "", lc.Addr, "bind address")
				assert.Equal(t, []string{lc.Name}, lc.Servers, "servers")
				sc := serverOf(conf, lc)
				require.NotNil(t, sc.TURN, "TURN server")
				assert.Equal(t, "default", sc.TURN.Auth, "auth policy")
				assert.Equal(t, "default", conf.Auths[0].Name, "auth policy name")

				// a server filter per cluster, admitting its peers; a server without clusters
				// has none, denying every peer
				udpSrv := "testnamespace/gateway-1/gateway-1-listener-udp"
				assert.Equal(t, []string{udpSrv + "/testnamespace/udproute-ok/udp"},
					sc.Filters, "server filters")
				sc = serverOf(conf, conf.Listeners[1])
				require.NotNil(t, sc.TURN, "TURN server")
				assert.Empty(t, sc.Filters, "server filters")

				assert.Len(t, conf.Filters, 1, "filter num")
				f, err := conf.GetFilterConfig(udpSrv + "/testnamespace/udproute-ok/udp")
				assert.NoError(t, err, "server filter found")
				assert.Len(t, f.Rules, 4, "server filter rules")
				for _, r := range f.Rules {
					assert.Equal(t, "ALLOW", r.Action, "server filter action")
					assert.Empty(t, r.Ports, "server filter ports")
				}

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
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
			dps:   []stnrgwv1.Dataplane{testutils.TestDataplane},
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
				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))
				assert.NoError(t, r.renderForGateways(c), "render success")

				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Len(t, conf.Clusters, 2, "cluster num")
				c1, err := conf.GetClusterConfig("testnamespace/udproute-ok/udp")
				assert.NoError(t, err, "cluster 1 found")
				assert.Empty(t, c1.Filters, "a cluster has no filter")
				c2, err := conf.GetClusterConfig("testnamespace/route-2/udp")
				assert.NoError(t, err, "cluster 2 found")
				assert.Empty(t, c2.Filters, "a cluster has no filter")
				assert.Equal(t, []string{"10.11.12.13", "10.11.12.14", "10.11.12.15"}, c2.Endpoints, "endpoints")

				// the filters of the server of the UDP listener: per cluster in order, the peers on
				// the port range of their backend
				lc, err := conf.GetListenerConfig("testnamespace/gateway-1/gateway-1-listener-udp")
				assert.NoError(t, err, "listener found")
				sc := serverOf(conf, lc)
				assert.Len(t, sc.Clusters, 2, "route num")
				pfs := []string{}
				for _, cn := range sc.Clusters {
					pfs = append(pfs, sc.Name+"/"+cn)
				}
				assert.Equal(t, pfs, sc.Filters, "server filters")

				f, err := conf.GetFilterConfig(sc.Name + "/testnamespace/udproute-ok/udp")
				assert.NoError(t, err, "server filter 1 found")
				assert.Len(t, f.Rules, 4, "server filter 1 rules")
				for _, r := range f.Rules {
					assert.Equal(t, "ALLOW", r.Action, "server filter action")
					assert.Equal(t, []stnrapiv2.FilterPort{{Port: 100, EndPort: 200}}, r.Ports, "server filter ports")
				}
				f, err = conf.GetFilterConfig(sc.Name + "/testnamespace/route-2/udp")
				assert.NoError(t, err, "server filter 2 found")
				assert.Len(t, f.Rules, 3, "server filter 2 rules")
				for _, r := range f.Rules {
					assert.Empty(t, r.Ports, "no port range, no ports")
				}

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name: "TURN and L4 listeners - E2E test",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("turn"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("l4"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}

				// attach to both listeners
				ro := testutils.TestUDPRoute.DeepCopy()
				ro.Spec.CommonRouteSpec.ParentRefs = []gwapiv1.ParentReference{{Name: "gateway-1"}}
				port := gwapiv1.PortNumber(5000)
				ro.Spec.Rules[0].BackendRefs[0].Port = &port
				c.rs = []stnrgwv1.UDPRoute{*ro}
			},
			tester: func(t *testing.T, r *renderer) {
				config.EnableRelayToClusterIP = false

				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				c.update = event.NewEventUpdate(0)
				c.gws.ResetGateways(r.getGateways4Class(c))
				assert.NoError(t, r.renderForGateways(c), "render success")

				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				// one cluster, shared by both servers
				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc, err := conf.GetClusterConfig("testnamespace/udproute-ok/udp")
				assert.NoError(t, err, "cluster")
				assert.Equal(t, []string{"$STUNNER_ADDR"}, rc.Addrs, "relay address")
				assert.Equal(t, []string{"1.2.3.4:5000", "1.2.3.5:5000"},
					rc.Endpoints, "endpoints at the backend port")

				turn, err := conf.GetListenerConfig("testnamespace/gateway-1/turn")
				assert.NoError(t, err, "TURN listener")
				ts := serverOf(conf, turn)
				assert.Equal(t, "TURN", ts.Type, "TURN server")
				assert.Equal(t, []string{"testnamespace/udproute-ok/udp"}, ts.Clusters, "TURN clusters")
				assert.Equal(t, []string{ts.Name + "/testnamespace/udproute-ok/udp"},
					ts.Filters, "server filters")

				l4, err := conf.GetListenerConfig("testnamespace/gateway-1/l4")
				assert.NoError(t, err, "L4 listener")
				assert.Equal(t, "UDP", l4.Protocol, "L4 listener protocol")
				ls := serverOf(conf, l4)
				assert.Equal(t, "L4", ls.Type, "L4 server")
				assert.Equal(t, "default", ls.L4.Auth, "L4 auth policy")
				assert.Equal(t, []string{"testnamespace/udproute-ok/udp"}, ls.Clusters, "L4 clusters")

				// the route resolves on both
				ros := r.allRoutes()
				assert.Len(t, ros, 1, "routes len")
				d := meta.FindStatusCondition(ros[0].GetRouteStatus().Parents[0].Conditions,
					string(gwapiv1.RouteConditionResolvedRefs))
				assert.NotNil(t, d, "resolved-refs found")
				assert.Equal(t, metav1.ConditionTrue, d.Status, "resolved")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
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
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
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

				// the new gateway-config in another namespace
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

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 2, "endpoints len")
				assert.Contains(t, rc.Endpoints, "1.2.3.4", "endpoint ip-1")
				assert.Contains(t, rc.Endpoints, "1.2.3.5", "endpoint ip-2")
				assert.NotContains(t, rc.Endpoints, "1.2.3.6", "endpoint ip-3: not ready, not dialed")
				assert.NotContains(t, rc.Endpoints, "1.2.3.7", "endpoint ip-4: not ready, not dialed")
				assert.Contains(t, serverFilterBlocks(conf, rc.Name), "4.3.2.1/32", "cluster-ip: a peer, no endpoint")

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

				// config-queue
				cs = c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf = cs[0]

				assert.Equal(t, "dummy-namespace/dummy-gateway", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				// the service links to the original gateway, our gateway does not
				// have linkage, so public addr should be empty
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "dummy-namespace/dummy-route/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "dummy-namespace/dummy-gateway/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc = conf.Clusters[0]
				assert.Equal(t, "dummy-namespace/dummy-route/udp", rc.Name, "cluster name")
				assert.Equal(t, "STATIC", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Contains(t, rc.Endpoints, "4.4.4.4", "endpoint ip-1")

				// restore
				config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			},
		},
		{
			name:  "StaticService - E2E test",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			svcs:  []corev1.Service{testutils.TestSvc},
			ssvcs: []stnrgwv1.StaticService{testutils.TestStaticSvc},
			dps:   []stnrgwv1.Dataplane{testutils.TestDataplane},
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

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

				assert.Len(t, conf.Clusters, 1, "cluster num")
				rc := conf.Clusters[0]
				assert.Equal(t, "testnamespace/udproute-ok/udp", rc.Name, "cluster name")
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
					gc.Status.Conditions[0].Reason, "conditions reason")
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
				assert.Len(t, ros, 1, "route num")
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

			},
		},
		{
			name: "Address hint set in Gw.spec.addresses",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{testutils.TestUDPRoute},
			svcs: []corev1.Service{testutils.TestSvc},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
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

				// config-queue
				cs := c.update.ConfigQueue
				assert.Len(t, cs, 1, "config ready")
				conf := cs[0]

				assert.Equal(t, "testnamespace/gateway-1", conf.Admin.Name, "name")
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
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.1.1.1", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 1, "route num")
				assert.Equal(t, serverOf(conf, lc).Clusters[0], "testnamespace/udproute-ok/udp", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "testnamespace/gateway-1/gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "TURN", serverOf(conf, lc).Type, "server type")
				assert.Equal(t, "1.1.1.1", lc.PublicAddr, "public-ip")
				assert.Len(t, serverOf(conf, lc).Clusters, 0, "route num")

			},
		},
	})
}
