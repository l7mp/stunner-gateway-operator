/*
Copyright 2022 The l7mp/stunner team.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner/v2/pkg/logger"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
	cdsclient "github.com/l7mp/stunner/v2/pkg/config/client"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

// findCluster returns the named cluster from a config, or nil.
func findCluster(c *stnrapiv1.StunnerConfig, name string) *stnrapiv1.ClusterConfig {
	for i := range c.Clusters {
		if c.Clusters[i].Name == name {
			return &c.Clusters[i]
		}
	}
	return nil
}

// listenerHasRoute reports whether the named listener of a config attaches the named route.
func listenerHasRoute(c *stnrapiv1.StunnerConfig, listener, route string) bool {
	for _, lc := range c.Listeners {
		if lc.Name != listener {
			continue
		}
		for _, r := range lc.Routes {
			if r == route {
				return true
			}
		}
	}
	return false
}

// routeAcceptedCondition returns the Accepted condition of the first parent status, or nil.
func routeAcceptedCondition(parents []gwapiv1.RouteParentStatus) *metav1.Condition {
	if len(parents) != 1 {
		return nil
	}
	return meta.FindStatusCondition(parents[0].Conditions,
		string(gwapiv1.RouteConditionAccepted))
}

// testTCPRouteLegacy exercises TCPRoute handling in legacy mode: the rendered ConfigMap carries a
// TCP-protocol cluster per TCPRoute, both the STUNner-native and the official Gateway API kinds
// render, statuses are set, and a same-name native route masks the official one.
func testTCPRouteLegacy() {
	Context("When creating TCPRoutes (LEGACY, EDS ENABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrapiv1.StunnerConfig{}
		tcpRouteGwAPI := testutils.TestTCPRouteV1.DeepCopy()
		tcpRouteGwAPI.SetName("tcproute-gwapi")

		BeforeAll(func() {
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true
		})

		AfterAll(func() {
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
		})

		It("should survive loading the base resources and a TCPRoute", func() {
			createOrUpdateGatewayClass(ctx, k8sClient, testGwClass, nil)
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
			createOrUpdateService(ctx, k8sClient, testSvc, nil)
			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)
			createOrUpdateTCPRoute(ctx, k8sClient, testTCPRoute, nil)
		})

		It("should render no TCP cluster without a license", func() {
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, cm); err != nil {
					return false
				}
				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}
				// the route attaches to the listener even though no cluster is
				// rendered for it
				if !listenerHasRoute(&c, "testnamespace/gateway-1/gateway-1-listener-tcp",
					"testnamespace/tcproute-ok") {
					return false
				}
				conf = &c
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(findCluster(conf, "testnamespace/tcproute-ok")).To(BeNil(),
				"no TCP cluster rendered")
		})

		It("should attach the TCPRoute to the TCP listener", func() {
			l := stnrapiv1.ListenerConfig{}
			found := false
			for _, lc := range conf.Listeners {
				if lc.Name == "testnamespace/gateway-1/gateway-1-listener-tcp" {
					l, found = lc, true
				}
			}
			Expect(found).Should(BeTrue(), "TCP listener found")
			Expect(l.Routes).Should(ContainElement("testnamespace/tcproute-ok"), "route attached")
		})

		It("should set the TCPRoute status", func() {
			ro := &stnrgwv1.TCPRoute{}
			lookupKey := store.GetNamespacedName(testTCPRoute)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, ro); err != nil {
					return false
				}
				d := routeAcceptedCondition(ro.Status.Parents)
				return d != nil && d.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			Expect(ro.Status.Parents).To(HaveLen(1), "parent status len")
			ps := ro.Status.Parents[0]
			Expect(string(ps.ParentRef.Name)).Should(Equal("gateway-1"), "parent name")

			d := meta.FindStatusCondition(ps.Conditions, string(gwapiv1.RouteConditionAccepted))
			Expect(d).NotTo(BeNil(), "accepted condition")
			Expect(d.Status).Should(Equal(metav1.ConditionTrue), "accepted")

			// the route is accepted, but the open-source renderer cannot resolve it
			// into a cluster
			d = meta.FindStatusCondition(ps.Conditions, string(gwapiv1.RouteConditionResolvedRefs))
			Expect(d).NotTo(BeNil(), "resolved-refs condition")
			Expect(d.Status).Should(Equal(metav1.ConditionFalse), "resolved-refs")
			Expect(d.Reason).Should(Equal(string(gwapiv1.RouteReasonUnsupportedValue)), "reason")
		})

		It("should render an official Gateway API TCPRoute", func() {
			createOrUpdateTCPRouteV1(ctx, k8sClient, tcpRouteGwAPI, nil)

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, cm); err != nil {
					return false
				}
				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}
				// no cluster without a license, but the route still attaches
				return listenerHasRoute(&c, "testnamespace/gateway-1/gateway-1-listener-tcp",
					"testnamespace/tcproute-gwapi") &&
					findCluster(&c, "testnamespace/tcproute-gwapi") == nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should set the official Gateway API TCPRoute status at v1", func() {
			ro := &gwapiv1.TCPRoute{}
			lookupKey := store.GetNamespacedName(tcpRouteGwAPI)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, ro); err != nil {
					return false
				}
				d := routeAcceptedCondition(ro.Status.Parents)
				return d != nil && d.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())
		})

		It("should mask the official TCPRoute with a same-name STUNner TCPRoute", func() {
			masking := testutils.TestTCPRoute.DeepCopy()
			masking.SetName("tcproute-gwapi")
			createOrUpdateTCPRoute(ctx, k8sClient, masking, nil)

			ro := &gwapiv1.TCPRoute{}
			lookupKey := store.GetNamespacedName(tcpRouteGwAPI)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, ro); err != nil {
					return false
				}
				d := routeAcceptedCondition(ro.Status.Parents)
				return d != nil && d.Status == metav1.ConditionFalse &&
					d.Reason == string(gwapiv1.RouteReasonPending)
			}, timeout, interval).Should(BeTrue())
		})

		It("should unmask the official TCPRoute when the STUNner TCPRoute is deleted", func() {
			masking := &stnrgwv1.TCPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "tcproute-gwapi",
				Namespace: string(testutils.TestNsName),
			}}
			Expect(k8sClient.Delete(ctx, masking)).Should(Succeed())

			ro := &gwapiv1.TCPRoute{}
			lookupKey := store.GetNamespacedName(tcpRouteGwAPI)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, ro); err != nil {
					return false
				}
				d := routeAcceptedCondition(ro.Status.Parents)
				return d != nil && d.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())
		})

		It("should survive deleting the TCPRoute resources", func() {
			ctrl.Log.Info("deleting the TCPRoutes")
			Expect(k8sClient.Delete(ctx, testTCPRoute)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, tcpRouteGwAPI)).Should(Succeed())

			ctrl.Log.Info("deleting the Gateway")
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())

			ctrl.Log.Info("deleting the Service")
			Expect(k8sClient.Delete(ctx, testSvc)).Should(Succeed())

			ctrl.Log.Info("deleting the EndpointSlice")
			Expect(k8sClient.Delete(ctx, testEndpointSlice)).Should(Succeed())
		})
	})
}

// testTCPRouteManaged exercises TCPRoute handling in managed mode via the CDS server.
func testTCPRouteManaged() {
	Context("When creating TCPRoutes (MANAGED, EDS ENABLED)", Ordered, Label("managed"), func() {
		var clientCtx context.Context
		var clientCancel context.CancelFunc
		var ch chan *stnrapiv1.StunnerConfig
		var cdsClient cdsclient.Client

		BeforeAll(func() {
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch = make(chan *stnrapiv1.StunnerConfig, 128)
			var err error
			log := logger.NewLoggerFactory(stunnerLogLevel)
			cdsClient, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", "", log)
			Expect(err).Should(Succeed())
			Expect(cdsClient.Watch(clientCtx, ch, false)).Should(Succeed())
		})

		AfterAll(func() {
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			clientCancel()
			// do not close(ch): the watcher may deliver an in-flight config after the
			// cancel and would panic on a closed channel; the channel is dropped with
			// the context
		})

		It("should survive loading the base resources and a TCPRoute", func() {
			createOrUpdateGatewayClass(ctx, k8sClient, testGwClass, nil)
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
			createOrUpdateService(ctx, k8sClient, testSvc, nil)
			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)
			createOrUpdateTCPRoute(ctx, k8sClient, testTCPRoute, nil)

			ctrl.Log.Info("loading default Dataplane")
			current := &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: testDataplane.GetName(),
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testDataplane.Spec.DeepCopyInto(&current.Spec)
				return nil
			})
			Expect(err).Should(Succeed())
		})

		It("should render no TCP cluster without a license", func() {
			Eventually(checkConfig(ch, func(c *stnrapiv1.StunnerConfig) bool {
				// the route attaches to the listener even though no cluster is
				// rendered for it
				return listenerHasRoute(c, "testnamespace/gateway-1/gateway-1-listener-tcp",
					"testnamespace/tcproute-ok") &&
					findCluster(c, "testnamespace/tcproute-ok") == nil
			}), timeout, interval).Should(BeTrue())
		})

		It("should set the TCPRoute status", func() {
			ro := &stnrgwv1.TCPRoute{}
			lookupKey := store.GetNamespacedName(testTCPRoute)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, ro); err != nil {
					return false
				}
				d := routeAcceptedCondition(ro.Status.Parents)
				return d != nil && d.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			// the route is accepted, but the open-source renderer cannot resolve it
			// into a cluster
			d := meta.FindStatusCondition(ro.Status.Parents[0].Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(d).NotTo(BeNil(), "resolved-refs condition")
			Expect(d.Status).Should(Equal(metav1.ConditionFalse), "resolved-refs")
			Expect(d.Reason).Should(Equal(string(gwapiv1.RouteReasonUnsupportedValue)), "reason")
		})

		It("should survive deleting the TCPRoute resources", func() {
			ctrl.Log.Info("deleting the TCPRoute")
			Expect(k8sClient.Delete(ctx, testTCPRoute)).Should(Succeed())

			ctrl.Log.Info("deleting the Gateway")
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())

			ctrl.Log.Info("deleting the Service")
			Expect(k8sClient.Delete(ctx, testSvc)).Should(Succeed())

			ctrl.Log.Info("deleting the EndpointSlice")
			Expect(k8sClient.Delete(ctx, testEndpointSlice)).Should(Succeed())
		})
	})
}
