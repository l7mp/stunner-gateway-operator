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
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
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
	testEndpoint = testutils.TestEndpoint.DeepCopy()
	_            = fmt.Sprintf("whatever: %d", 1)
)

// GatewayClass + GatewayConfig + Gateway should be enough to render a valid STUNner conf
var _ = Describe("Integration test:", func() {
	// fmt.Printf("%#v\n", testUDPRoute)
	// fmt.Printf("%#v\n", testSvc)

	// WITHOUT EDS
	Context("When creating a minimal set of API resources (EDS DISABLED)", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = false
			config.EnableRelayToClusterIP = false

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
				return err == nil
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
				return err == nil
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
				return err == nil

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

	Context("When re-loading the gateway and the route resources (EDS DISABLED)", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("should render a valid STUNner config", func() {

			ctrl.Log.Info("re-loading Gateway")
			recreateOrUpdateGateway(func(current *gatewayv1alpha2.Gateway) {})

			ctrl.Log.Info("re-loading UDPRoute")
			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {})

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

		It("should set STUNner config values correctly", func() {
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

		It("should reset status on all resources", func() {
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

			s = meta.FindStatusCondition(ps.Conditions,
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
	})

	Context("When changing a route parentref to the TCP listener (EDS DISABLED)", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}
		sn := gatewayv1alpha2.SectionName("gateway-1-listener-tcp")

		It("should render a valid STUNner config", func() {
			ctrl.Log.Info("re-loading UDPRoute")

			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {
				current.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
			})

			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				if len(c.Listeners[1].Routes) == 1 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

		It("should update the route for the listeners", func() {
			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			// fmt.Printf("1-%#v\n", conf)

			Expect(l.Name).Should(Equal("gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(0))

			l = conf.Listeners[1]
			if l.Name != "gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))
			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))
		})

		It("should reset status on all resources", func() {
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
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(sn)))
			Expect(ps.ControllerName).To(Equal(gatewayv1alpha2.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

			// Expect(k8sClient).To(BeNil())
		})
	})

	Context("The controller should dynamically render a new valid STUNner config (EDS DISABLED) when", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("changing the parentRef of a route", func() {

			ctrl.Log.Info("re-loading UDPRoute: ParentRef = dummy")
			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {
				current.Spec.CommonRouteSpec.ParentRefs[0].Name =
					gatewayv1alpha2.ObjectName("dummy")
			})

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

				if len(c.Listeners) != 2 || len(c.Clusters) == 0 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")

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
			Expect(l.Routes).To(HaveLen(0))

			l = conf.Listeners[1]
			if l.Name != "gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(0))

			Expect(conf.Clusters).To(HaveLen(0))

			ro := &gatewayv1alpha2.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Name).To(Equal(gatewayv1alpha2.ObjectName("dummy")))
			Expect(ps.ControllerName).To(Equal(gatewayv1alpha2.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gatewayv1alpha2.ConditionRouteAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ConditionRouteAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gatewayv1alpha2.ConditionRouteResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gatewayv1alpha2.ConditionRouteResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
		})

		It("changing the auth type", func() {
			atype := "longterm"
			secret := "dummy"

			ctrl.Log.Info("re-loading original UDPRoute")
			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
			})

			ctrl.Log.Info("re-loading gateway-config")
			recreateOrUpdateGatewayConfig(func(current *stunnerv1alpha1.GatewayConfig) {
				current.Spec.AuthType = &atype
				current.Spec.SharedSecret = &secret
			})

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

				if c.Auth.Type == "longterm" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Auth.Type).Should(Equal("longterm"))
			Expect(conf.Auth.Credentials["secret"]).Should(Equal("dummy"))
		})

		It("changing a listener port", func() {
			// the client may overwrite our objects, recreate!

			ctrl.Log.Info("re-loading gateway")
			recreateOrUpdateGateway(func(current *gatewayv1alpha2.Gateway) {
				current.Spec.Listeners[0].Port = gatewayv1alpha2.PortNumber(1234)
			})

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

				if len(c.Listeners) == 2 && c.Listeners[0].Port == 1234 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

		It("changing a route target", func() {
			ctrl.Log.Info("re-loading route")
			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {
				current.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gatewayv1alpha2.ObjectName("dummy")
			})

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

				if len(c.Clusters) == 1 && len(c.Clusters[0].Endpoints) == 1 &&
					c.Clusters[0].Endpoints[0] == "dummy.testnamespace.svc.cluster.local" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Clusters[0].Type).To(Equal("STRICT_DNS"))
		})

		It("adding a new route", func() {
			ctrl.Log.Info("re-loading the test route")
			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {})

			ctrl.Log.Info("adding a new route")
			current := &gatewayv1alpha2.UDPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "route-2",
				Namespace: testUDPRoute.GetNamespace(),
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
				current.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gatewayv1alpha2.ObjectName("dummy")
				current.Spec.Rules[0].BackendRefs = append(current.Spec.Rules[0].BackendRefs,
					gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Name: gatewayv1alpha2.ObjectName("dummy-2"),
						},
					})

				// try to attach to all listeners: p1 -> udp, p2 -> dummy, p3 -> tcp
				sn := gatewayv1alpha2.SectionName("invalid")
				current.Spec.CommonRouteSpec.ParentRefs = append(current.Spec.CommonRouteSpec.ParentRefs,
					gatewayv1alpha2.ParentRef{
						Name:        gatewayv1alpha2.ObjectName("gateway-1"),
						SectionName: &sn,
					})

				sn2 := gatewayv1alpha2.SectionName("gateway-1-listener-tcp")
				current.Spec.CommonRouteSpec.ParentRefs = append(current.Spec.CommonRouteSpec.ParentRefs,
					gatewayv1alpha2.ParentRef{
						Name:        gatewayv1alpha2.ObjectName("gateway-1"),
						SectionName: &sn2,
					})

				return nil
			})
			Expect(err).Should(Succeed())

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

				if len(c.Listeners) == 2 && len(c.Clusters) == 2 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")

			Expect(conf.Listeners).To(HaveLen(2))
			l := conf.Listeners[0]
			if l.Name != "gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Routes).To(HaveLen(2))
			Expect(l.Routes).Should(ContainElement("udproute-ok"))
			Expect(l.Routes).Should(ContainElement("route-2"))

			l = conf.Listeners[1]
			if l.Name != "gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes).Should(ContainElement("route-2"))

			Expect(conf.Clusters).To(HaveLen(2))

			c := conf.Clusters[0]
			if c.Name != "udproute-ok" {
				c = conf.Clusters[1]
			}

			Expect(c.Name).Should(Equal("udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints).Should(ContainElement("testservice-ok.testnamespace.svc.cluster.local"))

			c = conf.Clusters[1]
			if c.Name != "route-2" {
				c = conf.Clusters[0]
			}

			Expect(c.Name).Should(Equal("route-2"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(2))
			Expect(c.Endpoints).Should(ContainElement("dummy.testnamespace.svc.cluster.local"))
			Expect(c.Endpoints).Should(ContainElement("dummy-2.testnamespace.svc.cluster.local"))
		})

		It("adding a new gateway", func() {
			ctrl.Log.Info("re-loading the test gateway")

			recreateOrUpdateGateway(func(current *gatewayv1alpha2.Gateway) {
				current.Spec.Listeners[0].Port = gatewayv1alpha2.PortNumber(1234)
			})

			ctrl.Log.Info("adding a new gateway")
			current := &gatewayv1alpha2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: testutils.TestGw.GetNamespace(),
			}}

			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testutils.TestGw.Spec.DeepCopyInto(&current.Spec)

				current.Spec.Listeners[0].Name =
					gatewayv1alpha2.SectionName("gateway-2-udp")
				current.Spec.Listeners[0].Port =
					gatewayv1alpha2.PortNumber(1234)
				current.Spec.Listeners[2].Name =
					gatewayv1alpha2.SectionName("gateway-2-tcp")
				current.Spec.Listeners[2].Port =
					gatewayv1alpha2.PortNumber(4321)

				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("updating route-2 to point to both the old and the new gateway")
			ro := &gatewayv1alpha2.UDPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "route-2",
				Namespace: testUDPRoute.GetNamespace(),
			}}

			_, err = ctrlutil.CreateOrUpdate(ctx, k8sClient, ro, func() error {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&ro.Spec)

				ro.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gatewayv1alpha2.ObjectName("dummy")
				ro.Spec.Rules[0].BackendRefs = append(ro.Spec.Rules[0].BackendRefs,
					gatewayv1alpha2.BackendRef{
						BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
							Name: gatewayv1alpha2.ObjectName("dummy-2"),
						},
					})

				// try to attach to all listeners: p1 -> udp, p2 -> dummy, p3 -> tcp
				sn := gatewayv1alpha2.SectionName("gateway-2-udp")
				ro.Spec.CommonRouteSpec.ParentRefs = append(ro.Spec.CommonRouteSpec.ParentRefs,
					gatewayv1alpha2.ParentRef{
						Name:        gatewayv1alpha2.ObjectName("gateway-2"),
						SectionName: &sn,
					})

				sn2 := gatewayv1alpha2.SectionName("gateway-2-tcp")
				ro.Spec.CommonRouteSpec.ParentRefs = append(ro.Spec.CommonRouteSpec.ParentRefs,
					gatewayv1alpha2.ParentRef{
						Name:        gatewayv1alpha2.ObjectName("gateway-2"),
						SectionName: &sn2,
					})

				return nil
			})
			Expect(err).Should(Succeed())

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

				if len(c.Listeners) == 4 && len(c.Clusters) == 2 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")

			Expect(conf.Listeners).To(HaveLen(4))
			l := stunnerconfv1alpha1.ListenerConfig{}

			for _, _l := range conf.Listeners {
				if _l.Name == "gateway-1-listener-udp" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Routes).To(HaveLen(2))
			Expect(l.Routes).Should(ContainElement("udproute-ok"))
			Expect(l.Routes).Should(ContainElement("route-2"))

			for _, _l := range conf.Listeners {
				if _l.Name == "gateway-1-listener-tcp" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Routes).To(HaveLen(0))

			for _, _l := range conf.Listeners {
				if _l.Name == "gateway-2-udp" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("gateway-2-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1234))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes).Should(ContainElement("route-2"))

			for _, _l := range conf.Listeners {
				if _l.Name == "gateway-2-tcp" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("gateway-2-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(4321))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes).Should(ContainElement("route-2"))

			Expect(conf.Clusters).To(HaveLen(2))
			c := conf.Clusters[0]
			if c.Name != "udproute-ok" {
				c = conf.Clusters[1]
			}

			Expect(c.Name).Should(Equal("udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints).Should(ContainElement("testservice-ok.testnamespace.svc.cluster.local"))

			c = conf.Clusters[1]
			if c.Name != "route-2" {
				c = conf.Clusters[0]
			}

			Expect(c.Name).Should(Equal("route-2"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(2))
			Expect(c.Endpoints).Should(ContainElement("dummy.testnamespace.svc.cluster.local"))
			Expect(c.Endpoints).Should(ContainElement("dummy-2.testnamespace.svc.cluster.local"))
		})

		It("should survive a full cleanup", func() {
			ctrl.Log.Info("deleting Gateway")
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())

			ctrl.Log.Info("deleting UDPRoute")
			Expect(k8sClient.Delete(ctx, testUDPRoute)).Should(Succeed())

			ctrl.Log.Info("deleting gateway-2")
			gw := &gatewayv1alpha2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: testutils.TestGw.GetNamespace(),
			}}
			Expect(k8sClient.Delete(ctx, gw)).Should(Succeed())

			ctrl.Log.Info("deleting route-3")
			ro := &gatewayv1alpha2.UDPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "route-2",
				Namespace: testUDPRoute.GetNamespace(),
			}}
			Expect(k8sClient.Delete(ctx, ro)).Should(Succeed())

			// restore
			config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
		})

		It("should render an empty config", func() {
			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) == 0 && len(c.Clusters) == 0 {
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})
	})

	// WITH EDS, WITHOUT RELAY-CLUSTER-IP
	Context("When creating a minimal set of API resources (EDS ENABLED, RELAY-TO-CLUSTER-IP DISABLED)", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = false

			recreateOrUpdateGateway(func(current *gatewayv1alpha2.Gateway) {})
			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {})

			Expect(k8sClient.Create(ctx, testSvc)).Should(Succeed())
			Expect(k8sClient.Create(ctx, testEndpoint)).Should(Succeed())

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

				if len(c.Listeners) == 2 && len(c.Clusters) == 1 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should re-render STUNner config with one cluster", func() {
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
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(4))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))
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

			// restoure
			config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
		})
	})

	// WITH EDS and RELAY-CLUSTER-IP
	Context("When creating a minimal set of API resources (EDS ENABLED, RELAY-TO-CLUSTER-IP ENABLED)", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			// need to trigger a re-render: delete the invalid Gateway listener
			ctrl.Log.Info("re-loading Gateway with 1 valid listener")
			recreateOrUpdateGateway(func(current *gatewayv1alpha2.Gateway) {
				current.Spec.Listeners = []gatewayv1alpha2.Listener{
					current.Spec.Listeners[0], current.Spec.Listeners[1]}
			})

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

				if len(c.Listeners) == 1 && len(c.Clusters) == 1 &&
					c.Clusters[0].Type == "STATIC" &&
					len(c.Clusters[0].Endpoints) == 5 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should re-render STUNner config with one cluster with the Cluster-IP", func() {
			Expect(conf.Listeners).To(HaveLen(1))

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

			Expect(conf.Clusters).To(HaveLen(1))

			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))

			svc := store.Services.GetObject(types.NamespacedName{
				Namespace: "testnamespace", Name: "testservice-ok"})
			Expect(c.Endpoints).Should(ContainElement(svc.Spec.ClusterIP))

			config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = config.DefaultEnableRelayToClusterIP
		})
	})

	Context("When changing a route parentref to the TCP listener (EDS ENABLED)", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}
		sn := gatewayv1alpha2.SectionName("gateway-1-listener-tcp")

		It("should render a valid STUNner config", func() {
			ctrl.Log.Info("re-loading Gateway with 2 valid listeners")
			recreateOrUpdateGateway(func(current *gatewayv1alpha2.Gateway) {})

			ctrl.Log.Info("re-loading UDPRoute")
			recreateOrUpdateUDPRoute(func(current *gatewayv1alpha2.UDPRoute) {
				current.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
			})

			lookupKey := types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := testutils.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				if len(c.Listeners[1].Routes) == 1 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

		It("should update the route for the listeners", func() {
			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			// fmt.Printf("1-%#v\n", conf)

			Expect(l.Name).Should(Equal("gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(0))

			l = conf.Listeners[1]
			if l.Name != "gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))
			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))

			svc := store.Services.GetObject(types.NamespacedName{
				Namespace: "testnamespace", Name: "testservice-ok"})
			Expect(c.Endpoints).Should(ContainElement(svc.Spec.ClusterIP))
		})

		It("should reset status on all resources", func() {
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
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(sn)))
			Expect(ps.ControllerName).To(Equal(gatewayv1alpha2.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

			// Expect(k8sClient).To(BeNil())
		})

		It("should survive a full cleanup", func() {
			ctrl.Log.Info("deleting GatewayClass")
			Expect(k8sClient.Delete(ctx, testGwClass)).Should(Succeed())

			ctrl.Log.Info("deleting GatewayConfig")
			Expect(k8sClient.Delete(ctx, testGwConfig)).Should(Succeed())

			ctrl.Log.Info("deleting Gateway")
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())

			ctrl.Log.Info("deleting UDPRoute")
			Expect(k8sClient.Delete(ctx, testUDPRoute)).Should(Succeed())

			ctrl.Log.Info("deleting Service")
			Expect(k8sClient.Delete(ctx, testSvc)).Should(Succeed())

			// restore
			config.EnableEndpointDiscovery = config.DefaultEnableEndpointDiscovery
		})
	})
})
