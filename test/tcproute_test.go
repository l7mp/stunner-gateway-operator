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

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
	cdsclient "github.com/l7mp/stunner/v2/pkg/config/client"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

// findCluster returns the named cluster from a config, or nil.
func findCluster(c *stnrapiv2.StunnerConfig, name string) *stnrapiv2.ClusterConfig {
	for i := range c.Clusters {
		if c.Clusters[i].Name == name {
			return &c.Clusters[i]
		}
	}
	return nil
}

// listenerHasCluster reports whether the server of the named listener of a config names the
// named cluster.
func listenerHasCluster(c *stnrapiv2.StunnerConfig, listener, cluster string) bool {
	for _, lc := range c.Listeners {
		if lc.Name != listener {
			continue
		}
		for _, r := range serverOf(c, lc).Clusters {
			if r == cluster {
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
	Context("When creating TCPRoutes (LEGACY)", Ordered, Label("legacy"), func() {
		conf := &stnrapiv2.StunnerConfig{}
		tcpRouteGwAPI := testutils.TestTCPRouteV1.DeepCopy()
		tcpRouteGwAPI.SetName("tcproute-gwapi")

		It("should survive loading the base resources and a TCPRoute", func() {
			createOrUpdateGatewayClass(ctx, k8sClient, testGwClass, nil)
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
			createOrUpdateService(ctx, k8sClient, testSvc, nil)
			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)
			createOrUpdateTCPRoute(ctx, k8sClient, testTCPRoute, nil)
		})

		It("should render a TCP cluster", func() {
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
				if !listenerHasCluster(&c, "testnamespace/gateway-1/gateway-1-listener-tcp",
					"testnamespace/tcproute-ok/tcp") {
					return false
				}
				conf = &c
				return true
			}, timeout, interval).Should(BeTrue())

			rc := findCluster(conf, "testnamespace/tcproute-ok/tcp")
			Expect(rc).NotTo(BeNil(), "TCP cluster rendered")
			Expect(rc.Protocol).Should(Equal("TCP"), "TCP cluster")
		})

		It("should attach the TCPRoute to the TCP listener", func() {
			l := stnrapiv2.ListenerConfig{}
			found := false
			for _, lc := range conf.Listeners {
				if lc.Name == "testnamespace/gateway-1/gateway-1-listener-tcp" {
					l, found = lc, true
				}
			}
			Expect(found).Should(BeTrue(), "TCP listener found")
			Expect(serverOf(conf, l).Clusters).Should(ContainElement("testnamespace/tcproute-ok/tcp"), "route attached")
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

			d = meta.FindStatusCondition(ps.Conditions, string(gwapiv1.RouteConditionResolvedRefs))
			Expect(d).NotTo(BeNil(), "resolved-refs condition")
			Expect(d.Status).Should(Equal(metav1.ConditionTrue), "resolved-refs")
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
				return listenerHasCluster(&c, "testnamespace/gateway-1/gateway-1-listener-tcp",
					"testnamespace/tcproute-gwapi/tcp") &&
					findCluster(&c, "testnamespace/tcproute-gwapi/tcp") != nil
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
	Context("When creating TCPRoutes (MANAGED)", Ordered, Label("managed"), func() {
		var clientCtx context.Context
		var clientCancel context.CancelFunc
		var ch chan *stnrapiv2.StunnerConfig
		var cdsClient cdsclient.Client

		BeforeAll(func() {
			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch = make(chan *stnrapiv2.StunnerConfig, 128)
			var err error
			log := logger.NewLoggerFactory(stunnerLogLevel)
			cdsClient, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", nil, log)
			Expect(err).Should(Succeed())
			Expect(cdsClient.Watch(clientCtx, ch, false)).Should(Succeed())
		})

		AfterAll(func() {
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

		It("should render a TCP cluster", func() {
			Eventually(checkConfig(ch, func(c *stnrapiv2.StunnerConfig) bool {
				return listenerHasCluster(c, "testnamespace/gateway-1/gateway-1-listener-tcp",
					"testnamespace/tcproute-ok/tcp") &&
					findCluster(c, "testnamespace/tcproute-ok/tcp") != nil
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

			d := meta.FindStatusCondition(ro.Status.Parents[0].Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(d).NotTo(BeNil(), "resolved-refs condition")
			Expect(d.Status).Should(Equal(metav1.ConditionTrue), "resolved-refs")
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
