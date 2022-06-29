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
	// "time"
	// "reflect"
	// "testing"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	// apierrors "k8s.io/apimachinery/pkg/api/errors"
	// v1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// make sure we use fmt
var (
	testGwClass  = testutils.TestGwClass.DeepCopy()
	testGwConfig = testutils.TestGwConfig.DeepCopy()
	testGw       = testutils.TestGw.DeepCopy()
	testUDPRoute = testutils.TestUDPRoute.DeepCopy()
	testSvc      = testutils.TestSvc.DeepCopy()
	_            = fmt.Sprintf("whatever: %d", 1)
)

// GatewayClass + GatewayConfig + Gateway should be enough to render a valid STUNner conf
var _ = Describe("Integration test:", func() {
	conf := &stunnerconfv1alpha1.StunnerConfig{}

	// fmt.Printf("%#v\n", testUDPRoute)
	// fmt.Printf("%#v\n", testSvc)

	Context("When creating a minimal set of API resources the controller", func() {

		It("should survive loading a minimal config", func() {
			ctrl.Log.Info("loading GatewayClass")
			// fmt.Printf("%#v\n", testGwClass)
			Expect(k8sClient.Create(ctx, testGwClass)).Should(Succeed())
			ctrl.Log.Info("loading GatewayConfig")
			// fmt.Printf("%#v\n", testGwConfig)
			Expect(k8sClient.Create(ctx, testGwConfig)).Should(Succeed())
			ctrl.Log.Info("loading Gateway")
			// fmt.Printf("%#v\n", testGw)
			Expect(k8sClient.Create(ctx, testGw)).Should(Succeed())
		})

		It("should allow the gateway-config to be queried", func() {
			ctrl.Log.Info("reading back gateway-config")
			lookupKey := types.NamespacedName{
				Name:      testutils.TestGwConfig.GetName(),
				Namespace: testutils.TestGwConfig.GetNamespace(),
			}
			gwConfig := &stunnerv1alpha1.GatewayConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, gwConfig)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(gwConfig.GetName()).To(Equal(testutils.TestGwConfig.GetName()),
				"GatewayClass name")
			Expect(gwConfig.GetNamespace()).To(Equal(testutils.TestGwConfig.GetNamespace()),
				"GatewayClass namespace")
		})

		It("should successfully render a STUNner ConfigMap", func() {

			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				return true

			}, timeout, interval).Should(BeTrue())

			Expect(cm).NotTo(BeNil(), "STUNner config rendered")

		})

		It("should render a ConfigMap that can be successfully unpacked", func() {

			// retry, but also try to unpack inside Eventually
			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				_, err = testutils.UnpackConfigMap(cm)
				if err == nil {
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(cm).NotTo(BeNil(), "STUNner config rendered")

		})

		It("should render a STUNner config with exactly 2 listeners", func() {

			// retry, but also try to unpack inside Eventually
			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) == 2 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

		It("should render a STUNner config with correct listener params", func() {
			Expect(conf.Listeners).To(HaveLen(2))

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))

			l = conf.Listeners[1]
			if l.Name != "gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
		})

		It("should set the GatewayClass status", func() {
			gc := &gatewayv1alpha2.GatewayClass{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGwClass),
				gc)).Should(Succeed())

			Expect(gc.Status.Conditions).To(HaveLen(1))

			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gatewayv1alpha2.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gatewayv1alpha2.GatewayClassReasonAccepted)))

		})

		It("should set the Gateway status", func() {
			gw := &gatewayv1alpha2.Gateway{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw),
				gw)).Should(Succeed())

			Expect(gw.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw.Status.Conditions,
				string(gatewayv1alpha2.GatewayConditionScheduled))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.GatewayConditionScheduled)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw.Status.Conditions,
				string(gatewayv1alpha2.GatewayConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.GatewayConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			// listeners: no public gateway address so Ready status is False
			Expect(gw.Status.Listeners).To(HaveLen(3))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gatewayv1alpha2.ListenerConditionDetached))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionDetached)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonAttached)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gatewayv1alpha2.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gatewayv1alpha2.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonPending)))

			// listeners[1]: detached
			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gatewayv1alpha2.ListenerConditionDetached))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionDetached)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonUnsupportedProtocol)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gatewayv1alpha2.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gatewayv1alpha2.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonPending)))

			// listeners[2]: ok
			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gatewayv1alpha2.ListenerConditionDetached))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionDetached)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonAttached)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gatewayv1alpha2.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gatewayv1alpha2.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gatewayv1alpha2.ListenerReasonPending)))

		})

		It("should survive the event of adding a route", func() {
			ctrl.Log.Info("loading UDPRoute")
			// fmt.Printf("%#v\n", testUDPRoute)
			Expect(k8sClient.Create(ctx, testUDPRoute)).Should(Succeed())

			// retry, but also try to unpack inside Eventually
			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid cluster confs
				if len(c.Clusters) == 1 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())

		})

		It("should re-render STUNner config with the new cluster", func() {
			Expect(conf.Listeners).To(HaveLen(2))

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).Should(BeEmpty())

			Expect(conf.Clusters).To(HaveLen(1))

			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))
		})

		It("should set the Route status", func() {
			ro := &gatewayv1alpha2.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gatewayv1alpha2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gatewayv1alpha2.Kind("Gateway"))))
			// Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(gatewayv1alpha2.Namespace("testnamespace"))))
			// Expect(ps.ParentRef.Name).To(Equal(gatewayv1alpha2.ObjectName("gateway-1")))
			// Expect(ps.ControllerName).To(Equal(gatewayv1alpha2.GatewayController("gatewayclass-ok")))

			// Expect(ps.ParentRef.Group).To(BeNil())
			// Expect(ps.ParentRef.Kind).To(BeNil())
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gatewayv1alpha2.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(testutils.TestSectionName)))
			Expect(ps.ControllerName).To(Equal(gatewayv1alpha2.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gatewayv1alpha2.ConditionRouteAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ConditionRouteAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gatewayv1alpha2.ConditionRouteResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ConditionRouteResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

		// we cannot set the public IP: no load-balancer operator in the envTest API server
		//
		// It("should survive the event of adding a LoadBalancer service", func() {
		// 	ctrl.Log.Info("loading LoadBalancer service")
		// 	Expect(k8sClient.Create(ctx, &testutils.TestSvc)).Should(Succeed())

		// 	// retry, but also chjeck if a public address has been added
		// 	lookupKey := types.NamespacedName{
		// 		Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
		// 		Namespace: string(testutils.TestNs),
		// 	}
		// 	cm := &corev1.ConfigMap{}

		// 	ctrl.Log.Info("trying to Get STUNner configmap", "resource",
		// 		lookupKey)
		// 	Eventually(func() bool {
		// 		err := k8sClient.Get(ctx, lookupKey, cm)
		// 		if err != nil {
		// 			return false
		// 		}

		// 		c, err := testutils.UnpackConfigMap(cm)
		// 		if err != nil {
		// 			return false
		// 		}

		// 		// conf should have valid listener confs
		// 		if len(c.Listeners) != 1 || len(c.Clusters) != 1 {
		// 			return false
		// 		}

		// 		// the UDP listener should have a valid loadbalancer IP set
		// 		l := conf.Listeners[0]

		// 		// not sure about the order
		// 		if l.Name != "gateway-1-listener-udp" {
		// 			l = conf.Listeners[1]
		// 		}

		// 		if l.PublicAddr == "1.2.3.4" {
		// 			conf = &c
		// 			return true
		// 		}

		// 		return false

		// 	}, timeout, interval).Should(BeTrue())

		// })

		// It("should re-render STUNner config with the public IP added", func() {
		// 	Expect(conf.Listeners).To(HaveLen(2))

		// 	// not sure about the order
		// 	l := conf.Listeners[0]
		// 	if l.Name != "gateway-1-listener-udp" {
		// 		l = conf.Listeners[1]
		// 	}

		// 	Expect(l.Name).Should(Equal("gateway-1-listener-udp"))
		// 	Expect(l.Protocol).Should(Equal("UDP"))
		// 	Expect(l.Port).Should(Equal(1))
		// 	Expect(l.MinRelayPort).Should(Equal(1))
		// 	Expect(l.MaxRelayPort).Should(Equal(2))
		// 	Expect(l.PublicAddr).Should(Equal("1.2.3.4"))
		// 	Expect(l.Routes).To(HaveLen(1))
		// 	Expect(l.Routes[0]).Should(Equal("udproute-ok"))

		// 	l = conf.Listeners[1]
		// 	if l.Name != "gateway-1-listener-tcp" {
		// 		l = conf.Listeners[0]
		// 	}

		// 	Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
		// 	Expect(l.Protocol).Should(Equal("TCP"))
		// 	Expect(l.Port).Should(Equal(2))
		// 	Expect(l.MinRelayPort).Should(Equal(1))
		// 	Expect(l.MaxRelayPort).Should(Equal(2))
		// 	Expect(l.Routes).Should(BeEmpty())

		// 	Expect(conf.Clusters).To(HaveLen(1))

		// 	c := conf.Clusters[0]

		// 	Expect(c.Name).Should(Equal("udproute-ok"))
		// 	Expect(c.Type).Should(Equal("STRICT_DNS"))
		// 	Expect(c.Endpoints).To(HaveLen(1))
		// 	Expect(c.Endpoints[0]).Should(Equal("testservice-ok.dummy.svc.cluster.local"))
		// })

		It("should survive deleting the route", func() {
			ctrl.Log.Info("deleting Route")
			Expect(k8sClient.Delete(ctx, testUDPRoute)).Should(Succeed())

			// wait until configmap gets updated
			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have no valid listener confs
				if len(c.Clusters) == 0 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should re-render STUNner config with an empty cluster conf", func() {
			Expect(conf.Clusters).To(HaveLen(0))
		})

		It("should handle the deletion of the gateway", func() {
			ctrl.Log.Info("deleting Gateway")
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())

			// wait until configmap gets updated
			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have no valid listener confs
				if len(c.Listeners) == 0 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should re-render STUNner config with an empty listener/cluster conf", func() {
			Expect(conf.Listeners).To(HaveLen(0))
			Expect(conf.Clusters).To(HaveLen(0))
		})
	})

	Context("When dynamically changing a resource the controller", func() {

		It("should survive re-loading the gateway and the route", func() {
			// the client may overwrite our objects, recreate!
			testGw = testutils.TestGw.DeepCopy()
			testUDPRoute = testutils.TestUDPRoute.DeepCopy()

			ctrl.Log.Info("re-loading Gateway")
			// fmt.Printf("%#v\n", testGw)
			Expect(k8sClient.Create(ctx, testGw)).Should(Succeed())
			ctrl.Log.Info("re-loading UDPRoute")
			// fmt.Printf("%#v\n", testUDPRoute)
			Expect(k8sClient.Create(ctx, testUDPRoute)).Should(Succeed())
		})

		It("should render a valid STUNner ConfigMap", func() {
			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) == 2 && len(c.Clusters) == 1 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

	})
})
