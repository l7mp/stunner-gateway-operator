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
	// "context"

	"context"

	// "reflect"
	// "testing"
	// "fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	cdsclient "github.com/l7mp/stunner/pkg/config/client"
	"github.com/l7mp/stunner/pkg/logger"
)

func testManagedModeEndpointController() {
	// SINGLE GATEWAY
	Context("When creating a minimal set of API resources (ENDPOINT-CONTROLLER-ENABLED)", Ordered, Label("managed"), func() {
		var conf *stnrv1.StunnerConfig
		var clientCtx context.Context
		var clientCancel context.CancelFunc
		var ch chan *stnrv1.StunnerConfig
		var cdsClient cdsclient.Client

		BeforeAll(func() {
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch = make(chan *stnrv1.StunnerConfig, 128)
			var err error
			cdsClient, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", "",
				logger.NewLoggerFactory(stunnerLogLevel))
			Expect(err).Should(Succeed())
			Expect(cdsClient.Watch(clientCtx, ch, false)).Should(Succeed())
		})

		AfterAll(func() {
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP

			clientCancel()
			close(ch)
		})

		It("should survive loading a minimal config", func() {
			// switch EDS on
			createOrUpdateGatewayClass(testGwClass, nil)
			createOrUpdateGatewayConfig(testGwConfig, nil)
			createOrUpdateGateway(testGw, nil)

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

		It("should allow the gateway-config to be queried", func() {
			ctrl.Log.Info("reading back gateway-config")
			lookupKey := types.NamespacedName{
				Name:      testutils.TestGwConfig.GetName(),
				Namespace: testutils.TestGwConfig.GetNamespace(),
			}
			gwConfig := &stnrgwv1.GatewayConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, gwConfig)
				return err == nil
			}, timeout, interval).Should(BeTrue())
			Expect(gwConfig.GetName()).To(Equal(testutils.TestGwConfig.GetName()),
				"GatewayClass name")
			Expect(gwConfig.GetNamespace()).To(Equal(testutils.TestGwConfig.GetNamespace()),
				"GatewayClass namespace")
		})

		It("should render a STUNner config with exactly 2 listeners", func() {
			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) == 2 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())
			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

		It("should render a STUNner config with correct listener params", func() {
			Expect(conf.Listeners).To(HaveLen(2))

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TURN-TCP"))
			Expect(l.Port).Should(Equal(2))
		})

		It("should set the GatewayClass status", func() {
			gc := &gwapiv1.GatewayClass{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGwClass), gc)
				if err != nil {
					return false
				}
				s := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				return s != nil && s.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			Expect(gc.Status.Conditions).To(HaveLen(1))

			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gwapiv1.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gwapiv1.GatewayClassReasonAccepted)))
		})

		It("should set the Gateway status", func() {
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				if s == nil || s.Status != metav1.ConditionFalse {
					// should not be programmed: tcp listener has no public ip
					return false
				}

				if len(gw.Status.Listeners) != 2 {
					return false
				}

				s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				return s != nil && s.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			Expect(gw.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))

			// listeners: no public gateway address so Ready status is False
			Expect(gw.Status.Listeners).To(HaveLen(2))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionConflicted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonNoConflicts)))

			// listeners[1]: ok
			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1.ListenerConditionConflicted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonNoConflicts)))
		})

		It("should survive the event of adding a route", func() {
			ctrl.Log.Info("loading UDPRoute")
			createOrUpdateUDPRoute(testUDPRoute, nil)

			ctrl.Log.Info("loading backend Service")
			createOrUpdateService(testSvc, nil)
			createOrUpdateEndpoints(testEndpoint, nil)

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				if len(c.Clusters) == 1 && len(c.Clusters[0].Endpoints) == 5 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())
		})

		It("should re-render STUNner config with the new cluster", func() {
			Expect(conf.Listeners).To(HaveLen(2))

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TURN-TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).Should(BeEmpty())

			Expect(conf.Clusters).To(HaveLen(1))

			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))
		})

		It("should set the Route status", func() {
			ro := &stnrgwv1.UDPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute), ro)
				if err != nil || len(ro.Status.Parents) != 1 {
					return false
				}

				// backend refs should be resolved
				s := meta.FindStatusCondition(ro.Status.Parents[0].Conditions,
					string(gwapiv1.RouteConditionResolvedRefs))
				return s != nil && s.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))

			// Expect(ps.ParentRef.Group).To(BeNil())
			// Expect(ps.ParentRef.Kind).To(BeNil())
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(testutils.TestSectionName)))
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

		It("should survive a full cleanup", func() {
			ctrl.Log.Info("deleting GatewayClass")
			Expect(k8sClient.Delete(ctx, testGwClass)).Should(Succeed())

			ctrl.Log.Info("deleting GatewayConfig")
			Expect(k8sClient.Delete(ctx, testGwConfig)).Should(Succeed())

			ctrl.Log.Info("deleting Gateway")
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())

			ctrl.Log.Info("deleting Route")
			Expect(k8sClient.Delete(ctx, testUDPRoute)).Should(Succeed())

			ctrl.Log.Info("deleting Service")
			Expect(k8sClient.Delete(ctx, testSvc)).Should(Succeed())

			// service delete deletes its enpoint
			// ctrl.Log.Info("deleting Endpoint")
			// Expect(k8sClient.Delete(ctx, testEndpoint)).Should(Succeed())

			ctrl.Log.Info("deleting Dataplane")
			Expect(k8sClient.Delete(ctx, testDataplane)).Should(Succeed())
		})
	})
}
