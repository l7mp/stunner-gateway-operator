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
	// "fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func testLegacyMode() {
	// WITHOUT EDS
	Context("When creating a minimal set of API resources (EDS DISABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = false
			config.EnableRelayToClusterIP = false

			createOrUpdateGatewayClass(ctx, k8sClient, testGwClass, nil)
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
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

		It("should successfully render a STUNner ConfigMap", func() {
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			Expect(cm).NotTo(BeNil(), "STUNner config rendered")
			_, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			v, ok := cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
		})

		It("should render a ConfigMap that can be successfully unpacked", func() {
			// retry, but also try to unpack inside Eventually
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				_, err = store.UnpackConfigMap(cm)
				return err == nil

			}, timeout, interval).Should(BeTrue())

			Expect(cm).NotTo(BeNil(), "STUNner config rendered")
			_, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			v, ok := cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
		})

		It("should render a STUNner config with exactly 2 listeners", func() {
			// retry, but also try to unpack inside Eventually
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
			_, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			v, ok := cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
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
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				// no public IP yet
				return s != nil && s.Status == metav1.ConditionFalse &&
					s.Reason == string(gwapiv1.GatewayReasonAddressNotAssigned)

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
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, nil)

			ctrl.Log.Info("loading backend Service")
			createOrUpdateService(ctx, k8sClient, testSvc, nil)

			ctrl.Log.Info("loading backend EndpointSlice")
			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)

			// retry, but also try to unpack inside Eventually
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid cluster confs
				if len(c.Clusters) == 1 && c.Clusters[0].Type == "STRICT_DNS" {
					conf = &c
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			_, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			v, ok := cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
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
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))
		})

		It("should set the Route status", func() {
			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			// Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(gwapiv1.Namespace("testnamespace"))))
			// Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			// Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController("gatewayclass-ok")))

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

		It("should allow Gateway to set the Gateway Address", func() {
			ctrl.Log.Info("re-loading gateway with specific address")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				addr := gwapiv1.IPAddressType
				current.Spec.Addresses = []gwapiv1.GatewayAddress{{
					Type:  &addr,
					Value: "1.2.3.5",
				}}
			})

			// retry, but also check if a public address has been added
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.5" && c.Listeners[0].PublicPort == 1 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		// we cannot set the public IP: no load-balancer operator in the envTest API server
		// -> check the nodeport fallback

		It("should install a NodePort public IP/port", func() {
			ctrl.Log.Info("re-loading gateway")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {})

			ctrl.Log.Info("loading a Kubernetes Node")
			createOrUpdateNode(ctx, k8sClient, testNode, nil)

			// retry, but also check if a public address has been added
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.4" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should update the public IP/port when node External IP changes", func() {
			ctrl.Log.Info("re-loading Node with new External IP")
			statusUpdateNode(ctx, k8sClient, "testnode-ok", func(current *corev1.Node) {
				current.Status.Addresses[1].Address = "4.3.2.1"
			})

			// retry, but also check if a public address has been added
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "4.3.2.1" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should remove the public IP/port when the node's External IP disappears", func() {
			ctrl.Log.Info("re-loading Node with no External IP")
			statusUpdateNode(ctx, k8sClient, "testnode-ok", func(current *corev1.Node) {
				current.Status.Addresses[1].Type = corev1.NodeInternalIP
			})

			// retry, but also check if a public address has been added
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should find new public IP/port when a new node with a working External IP appears", func() {
			ctrl.Log.Info("adding new Node with working External IP")
			current := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name: "node-2",
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testutils.TestNode.Spec.DeepCopyInto(&current.Spec)
				testutils.TestNode.Status.DeepCopyInto(&current.Status)
				return nil
			})
			Expect(err).Should(Succeed())

			// retry, but also check if a public address has been added
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.4" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should remove the public IP/port when the exposed LoadBalancer service type changes to ClusterIP", func() {
			ctrl.Log.Info("re-loading gateway-config with annotation: service-type: ClusterIP")
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, func(current *stnrgwv1.GatewayConfig) {
				current.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				current.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
			})

			// retry, but also check if a public address has been added
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should restore the public IP/port when the exposed LoadBalancer service type changes to NodePort", func() {
			ctrl.Log.Info("re-loading gateway with annotation: service-type: NodePort")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{opdefault.ServiceTypeAnnotationKey: "NodePort"})
			})

			// retry, but also check if a public address has been added
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.4" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should add annotations from Gateway", func() {
			ctrl.Log.Info("re-loading gateway with further annotations")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
					"someAnnotation":                   "dummy-1",
					"someOtherAnnotation":              "dummy-2",
				})
			})

			// retry, but also check if a public address has been added
			lookupKey := store.GetNamespacedName(testGw)
			svc := &corev1.Service{}
			Eventually(func() bool {
				svc = &corev1.Service{}
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}

				as := svc.GetAnnotations()
				a1, ok1 := as[opdefault.ServiceTypeAnnotationKey]
				a2, ok2 := as["someAnnotation"]
				a3, ok3 := as["someOtherAnnotation"]

				if ok1 && ok2 && ok3 && a1 == "NodePort" && a2 == "dummy-1" && a3 == "dummy-2" {
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			v, ok := svc.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
		})

		It("should retain externally set labels/annotations on the LoadBalancer service", func() {
			ctrl.Log.Info("re-loading service with new labels/annotations")
			svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name:      testGw.GetName(),
				Namespace: testGw.GetNamespace(),
			}}

			_, err := createOrUpdate(ctx, k8sClient, svc, func() error {
				// rewrite annotations and labels
				svc.SetLabels(map[string]string{
					"someLabel":      "some-label-val",
					"someOtherLabel": "some-other-label-val",
					// this cannot be removed, otherwise the watcher ignores the service
					opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
				})
				svc.SetAnnotations(map[string]string{
					"someNewAnnotation":      "some-ann-val",
					"someOtherNewAnnotation": "some-other-ann-val",
				})

				return nil
			})
			Expect(err).Should(Succeed())

			lookupKey := store.GetNamespacedName(testGw)
			Eventually(func() bool {
				svc = &corev1.Service{}
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}

				ls := svc.GetLabels()
				l1, ok1 := ls["someLabel"]
				l2, ok2 := ls["someOtherLabel"]
				l3, ok3 := ls[opdefault.OwnedByLabelKey]

				if !ok1 || !ok2 || !ok3 {
					return false
				}

				if l1 != "some-label-val" || l2 != "some-other-label-val" ||
					l3 != opdefault.OwnedByLabelValue {
					return false
				}

				as := svc.GetAnnotations()
				a1, ok1 := as["someNewAnnotation"]
				a2, ok2 := as["someOtherNewAnnotation"]
				a3, ok3 := as[opdefault.RelatedGatewayKey]

				if !ok1 || !ok2 || !ok3 {
					return false
				}

				if a1 != "some-ann-val" || a2 != "some-other-ann-val" ||
					a3 != store.GetObjectKey(testGw) {
					return false
				}

				return true

			}, timeout, interval).Should(BeTrue())
		})

		It("should not change NodePort when Gateway annotations are modified", func() {
			lookupKey := store.GetNamespacedName(testGw)

			// learn nodeports
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, lookupKey, svc)).Should(Succeed())
			Expect(svc.Spec.Ports).To(HaveLen(1))
			np1 := svc.Spec.Ports[0].NodePort
			// np1, np2, np3 := svc.Spec.Ports[0].NodePort,
			// 	svc.Spec.Ports[1].NodePort, svc.Spec.Ports[2].NodePort

			ctrl.Log.Info("re-loading gateway with further annotations")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
					"someAnnotation":                   "new-dummy-1",
					"someOtherAnnotation":              "dummy-2",
				})
			})

			// retry, but also check if a public address has been added
			Eventually(func() bool {
				svc = &corev1.Service{}
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}

				as := svc.GetAnnotations()
				a1, ok1 := as[opdefault.ServiceTypeAnnotationKey]
				a2, ok2 := as["someAnnotation"]
				a3, ok3 := as["someOtherAnnotation"]

				if ok1 && ok2 && ok3 && a1 == "NodePort" && a2 == "new-dummy-1" && a3 == "dummy-2" {
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			// query svc again
			Expect(k8sClient.Get(ctx, lookupKey, svc)).Should(Succeed())
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].NodePort).Should(Equal(np1))
			// Expect(svc.Spec.Ports[1].NodePort).Should(Equal(np2))
			// Expect(svc.Spec.Ports[2].NodePort).Should(Equal(np3))

			v, ok := svc.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
		})

		It("should install TLS cert/keys", func() {
			ctrl.Log.Info("loading TLS Secret")
			createOrUpdateSecret(ctx, k8sClient, testSecret, nil)

			ctrl.Log.Info("re-loading gateway with TLS cert/key the 2nd listener")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}

				current.Spec.Listeners[1].Name = gwapiv1.SectionName("gateway-1-listener-dtls")
				current.Spec.Listeners[1].Protocol = gwapiv1.ProtocolType("TURN-DTLS")
				current.Spec.Listeners[1].TLS = &tls
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[1].Cert != "" && c.Listeners[1].Key != "" {
					conf = &c
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(2))
			l := conf.Listeners[0]

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Cert).Should(Equal(testutils.TestCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should update TLS cert when Secret changes", func() {
			ctrl.Log.Info("re-loading TLS Secret")
			createOrUpdateSecret(ctx, k8sClient, testSecret, func(current *corev1.Secret) {
				current.Data["tls.crt"] = []byte("newcert")
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[1].Cert == newCert64 {
					conf = &c
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(2))
			l := conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Cert).Should(Equal(newCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should update TLS key when Secret changes", func() {
			ctrl.Log.Info("re-loading TLS Secret")
			createOrUpdateSecret(ctx, k8sClient, testSecret, func(current *corev1.Secret) {
				current.Data["tls.key"] = []byte("newkey")
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[1].Key == newKey64 {
					conf = &c
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(2))
			l := conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Cert).Should(Equal(testutils.TestCert64))
			Expect(l.Key).Should(Equal(newKey64))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should survive installing a TLS cert/key for multiple TLS/DTLS listeners", func() {
			ctrl.Log.Info("re-loading TLS Secret with restored cert/key")
			createOrUpdateSecret(ctx, k8sClient, testSecret, nil)

			ctrl.Log.Info("re-loading gateway with TLS cert/key the 1st listener")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
					TLS:      nil,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"),
					TLS:      &tls,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-tls"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-TLS"),
					TLS:      &tls,
				}, {
					Name:     gwapiv1.SectionName("invalid"),
					Port:     gwapiv1.PortNumber(4),
					Protocol: gwapiv1.ProtocolType("dummy"),
				}}
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 3 || len(c.Clusters) != 1 {
					return false
				}

				// certs/keys should be installed on the last 2 listeners
				if c.Listeners[1].Cert != "" && c.Listeners[1].Key != "" &&
					c.Listeners[2].Cert != "" && c.Listeners[2].Key != "" {
					conf = &c
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(3))
			l := conf.Listeners[0]

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Cert).Should(Equal(""))
			Expect(l.Key).Should(Equal(""))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(3))
			Expect(l.Cert).Should(Equal(testutils.TestCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())

			l = conf.Listeners[2]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tls"))
			Expect(l.Protocol).Should(Equal("TURN-TLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Cert).Should(Equal(testutils.TestCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should set the Gateway status", func() {
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				return len(gw.Status.Listeners) == 4
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
			Expect(gw.Status.Listeners).To(HaveLen(4))

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

			// listeners[2]: ok
			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1.ListenerConditionConflicted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonNoConflicts)))

			// listeners[3]: detached
			s = meta.FindStatusCondition(gw.Status.Listeners[3].Conditions,
				string(gwapiv1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonUnsupportedProtocol)))

			s = meta.FindStatusCondition(gw.Status.Listeners[3].Conditions,
				string(gwapiv1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[3].Conditions,
				string(gwapiv1.ListenerConditionConflicted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonNoConflicts)))
		})

		It("should survive converting the route to a StaticService backend", func() {
			ctrl.Log.Info("adding static service")
			createOrUpdateStaticService(ctx, k8sClient, testStaticSvc, nil)

			ctrl.Log.Info("reseting gateway")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
					TLS:      nil,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"), // make sure exposed even if mixed-proto-lb is disabled
					TLS:      &tls,
				}}
			})

			ctrl.Log.Info("updating Route")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, func(current *stnrgwv1.UDPRoute) {
				group := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
				kind := gwapiv1.Kind("StaticService")
				current.Spec.CommonRouteSpec = gwapiv1.CommonRouteSpec{
					ParentRefs: []gwapiv1.ParentReference{{
						Name: "gateway-1",
					}},
				}
				current.Spec.Rules[0].BackendRefs = []stnrgwv1.BackendRef{{
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
					},
				}}
			})

			// wait until configmap gets updated
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)

			contains := func(strs []string, val string) bool {
				for _, s := range strs {
					if s == val {
						return true
					}
				}
				return false
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Clusters) == 1 && contains(c.Clusters[0].Endpoints, "10.11.12.13") &&
					len(c.Listeners) == 2 && len(c.Listeners[0].Routes) == 1 && len(c.Listeners[1].Routes) == 1 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("should render a correct STUNner config", func() {
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))

			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(3))
			Expect(c.Endpoints).Should(ContainElement("10.11.12.13"))
			Expect(c.Endpoints).Should(ContainElement("10.11.12.14"))
			Expect(c.Endpoints).Should(ContainElement("10.11.12.15"))
		})

		It("should set the status correctly", func() {
			gc := &gwapiv1.GatewayClass{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGwClass),
				gc)).Should(Succeed())

			Expect(gc.Status.Conditions).To(HaveLen(1))

			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gwapiv1.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gwapiv1.GatewayClassReasonAccepted)))

			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue &&
					s.Reason == string(gwapiv1.GatewayReasonProgrammed)

			}, timeout, interval).Should(BeTrue())

			Expect(gw.Status.Conditions).To(HaveLen(2))

			s = meta.FindStatusCondition(gw.Status.Conditions,
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
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

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

			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(BeNil())
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

		It("should survive deleting the route", func() {
			ctrl.Log.Info("deleting StaticService")
			Expect(k8sClient.Delete(ctx, testStaticSvc)).Should(Succeed())

			ctrl.Log.Info("deleting Route")
			Expect(k8sClient.Delete(ctx, testUDPRoute)).Should(Succeed())

			// wait until configmap gets updated
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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

	Context("When re-loading the gateway and the route resources (EDS DISABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("should render a valid STUNner config", func() {
			ctrl.Log.Info("re-loading Gateway")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				})

				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
					TLS:      nil,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"), // make sure exposed even if mixed-proto-lb is disabled
					TLS:      &tls,
				}}
			})

			ctrl.Log.Info("re-loading UDPRoute")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, nil)

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				conf = &c

				l := conf.Listeners[0]
				if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
					l = conf.Listeners[1]
				}

				if len(l.Routes) != 1 {
					return false
				}

				return true
			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

		It("should set STUNner config values correctly", func() {
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).Should(BeEmpty())

			Expect(conf.Clusters).To(HaveLen(1))

			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))
		})

		It("should reset status on all resources", func() {
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue

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
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

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

			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(testutils.TestSectionName)))
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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
	})

	Context("When changing a route parentref to the DTLS listener (EDS DISABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}
		sn := gwapiv1.SectionName("gateway-1-listener-dtls")

		It("should render a valid STUNner config", func() {
			ctrl.Log.Info("re-loading UDPRoute")

			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, func(current *stnrgwv1.UDPRoute) {
				current.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(0))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))
			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))
		})

		It("should reset status on all resources", func() {
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue

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
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

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

			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(sn)))
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

			// Expect(k8sClient).To(BeNil())
		})
	})

	Context("When changing a gateway namespace attachment policy to All (EDS DISABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}
		// snudp := gwapiv1.SectionName("gateway-1-listener-udp")
		// sntcp := gwapiv1.SectionName("gateway-1-listener-tcp")

		It("should be possible to add a new route in a new namespace", func() {
			ctrl.Log.Info("creating a dummy-namespace")
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-namespace",
					Labels: map[string]string{
						"dummy-label":           "dummy-value",
						testutils.TestLabelName: "dummy-value",
					},
				},
			})).Should(Succeed())

			ctrl.Log.Info("recreating UDPRoute with open listener attachment")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, func(current *stnrgwv1.UDPRoute) {
				current.Spec.CommonRouteSpec.ParentRefs[0].SectionName = nil
			})

			ctrl.Log.Info("creating new UDPRoute in the dummy-namespace")
			ro := &stnrgwv1.UDPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-namespace-route",
				Namespace: "dummy-namespace",
			}}

			_, err := createOrUpdate(ctx, k8sClient, ro, func() error {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&ro.Spec)
				ro.Spec.CommonRouteSpec.ParentRefs[0].Name = "gateway-1"
				ro.Spec.CommonRouteSpec.ParentRefs[0].Namespace = &testutils.TestNsName
				ro.Spec.CommonRouteSpec.ParentRefs[0].SectionName = nil
				return nil
			})
			Expect(err).Should(Succeed())
		})

		It("should render a valid STUNner config", func() {
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				if len(c.Listeners[0].Routes) == 1 && len(c.Listeners[1].Routes) == 1 {
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))
			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))
		})

		It("should reset Gateway statuses", func() {
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue

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
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

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

		It("should reset Route statuses", func() {
			// the original UDPRoute
			ro := &stnrgwv1.UDPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute), ro)
				if err != nil {
					return false
				}

				// should be programmed
				return len(ro.Status.Parents) == 1
			}, timeout, interval).Should(BeTrue())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			// Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(gwapiv1.Namespace("testnamespace"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			// Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(snudp)))
			Expect(ps.ParentRef.SectionName).To(BeNil())
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

			// the new UDPRoute in the dummy-namespace
			ro = &stnrgwv1.UDPRoute{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Namespace: "dummy-namespace",
					Name:      "dummy-namespace-route",
				}, ro); err != nil {
					return false
				}

				// should be programmed
				return len(ro.Status.Parents) == 1
			}, timeout, interval).Should(BeTrue())

			// no listener accepts the route
			Expect(ro.Status.Parents).To(HaveLen(1))
			ps = ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(testutils.TestNsName)))
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(BeNil())
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

		It("should be possible to change the namespace attachment policy to All", func() {
			ctrl.Log.Info("updating the gateway: set namespace attachment policy to All for the 3rd listener")
			fromNamespaces := gwapiv1.NamespacesFromAll
			routeNamespaces := gwapiv1.RouteNamespaces{
				From: &fromNamespaces,
			}
			allowedRoutes := gwapiv1.AllowedRoutes{
				Namespaces: &routeNamespaces,
			}
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				})

				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
					TLS:      nil,
				}, {
					Name:          gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:          gwapiv1.PortNumber(2),
					Protocol:      gwapiv1.ProtocolType("TURN-DTLS"), // exposed even if mixed-proto-lb is disabled
					TLS:           &tls,
					AllowedRoutes: &allowedRoutes,
				}}
			})
		})

		It("should render a valid STUNner config", func() {
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) != 2 || len(c.Clusters) != 2 {
					return false
				}

				if len(c.Listeners[0].Routes) == 1 && len(c.Listeners[1].Routes) == 2 {
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(2))
			Expect(l.Routes).Should(ContainElement("testnamespace/udproute-ok"))
			Expect(l.Routes).Should(ContainElement("dummy-namespace/dummy-namespace-route"))

			Expect(conf.Clusters).To(HaveLen(2))
			c := conf.Clusters[0]
			if c.Name != "testnamespace/udproute-ok" {
				c = conf.Clusters[1]
			}

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))

			c = conf.Clusters[1]
			if c.Name != "dummy-namespace/dummy-namespace-route" {
				c = conf.Clusters[0]
			}

			Expect(c.Name).Should(Equal("dummy-namespace/dummy-namespace-route"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.dummy-namespace.svc.cluster.local"))
		})

		It("should reset status on all resources", func() {
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue

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
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

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

			// the original UDPRoute
			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(BeNil())
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

			// the new UDPRoute in the dummy-namespace
			ro = &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: "dummy-namespace", Name: "dummy-namespace-route"},
				ro)).Should(Succeed())

			// no listener accepts the route
			Expect(ro.Status.Parents).To(HaveLen(1))
			ps = ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(testutils.TestNsName)))
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(BeNil())
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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
	})

	Context("When changing a gateway namespace attachment policy to Selector (EDS DISABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("should be possible to change the namespace attachment policy to Selector", func() {
			ctrl.Log.Info("recreating UDPRoute with multiple parentrefs")
			sn := gwapiv1.SectionName("gateway-1-listener-dtls")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, func(current *stnrgwv1.UDPRoute) {
				current.Spec.CommonRouteSpec.ParentRefs = []gwapiv1.ParentReference{
					{
						Name:        "gateway-1",
						SectionName: &testutils.TestSectionName,
					},
					{
						Name:        "gateway-1",
						SectionName: &sn,
					},
				}
			})

			ctrl.Log.Info("updating the gateway: set namespace attachment policy for the listeners")
			fromNamespaces := gwapiv1.NamespacesFromSelector
			routeNamespaces1 := gwapiv1.RouteNamespaces{
				From: &fromNamespaces,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"invalid-label": "invalid-value"},
				},
			}

			allowedRoutes1 := gwapiv1.AllowedRoutes{
				Namespaces: &routeNamespaces1,
			}

			routeNamespaces2 := gwapiv1.RouteNamespaces{
				From: &fromNamespaces,
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      testutils.TestLabelName,
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{testutils.TestLabelValue, "dummy-value"},
						},
					},
				},
			}

			allowedRoutes2 := gwapiv1.AllowedRoutes{
				Namespaces: &routeNamespaces2,
			}
			createOrUpdateGateway(ctx, k8sClient, &testutils.TestGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				})

				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:          gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:          gwapiv1.PortNumber(1),
					Protocol:      gwapiv1.ProtocolType("TURN-UDP"),
					AllowedRoutes: &allowedRoutes1,
				}, {
					Name:          gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:          gwapiv1.PortNumber(2),
					Protocol:      gwapiv1.ProtocolType("TURN-DTLS"), // exposed even if mixed-proto-lb is disabled
					TLS:           &tls,
					AllowedRoutes: &allowedRoutes2,
				}}
			})
		})

		It("should render a valid STUNner config", func() {
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) != 2 || len(c.Clusters) != 2 {
					return false
				}

				if len(c.Listeners[0].Routes) == 0 && len(c.Listeners[1].Routes) == 2 {
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(0))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(2))
			Expect(l.Routes).Should(ContainElement("testnamespace/udproute-ok"))
			Expect(l.Routes).Should(ContainElement("dummy-namespace/dummy-namespace-route"))

			Expect(conf.Clusters).To(HaveLen(2))
			c := conf.Clusters[0]
			if c.Name != "testnamespace/udproute-ok" {
				c = conf.Clusters[1]
			}

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.testnamespace.svc.cluster.local"))

			c = conf.Clusters[1]
			if c.Name != "dummy-namespace/dummy-namespace-route" {
				c = conf.Clusters[0]
			}

			Expect(c.Name).Should(Equal("dummy-namespace/dummy-namespace-route"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints[0]).Should(Equal("testservice-ok.dummy-namespace.svc.cluster.local"))
		})

		It("should reset status on all resources", func() {
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue

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
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

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

			// the original UDPRoute
			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(2))
			ps := ro.Status.Parents[0]
			Expect(ps.ParentRef.SectionName).NotTo(BeNil())
			if *ps.ParentRef.SectionName != testutils.TestSectionName {
				ps = ro.Status.Parents[1]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).NotTo(BeNil())
			Expect(*ps.ParentRef.SectionName).To(Equal(testutils.TestSectionName))
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			ps = ro.Status.Parents[1]
			Expect(ps.ParentRef.SectionName).NotTo(BeNil())
			if *ps.ParentRef.SectionName == testutils.TestSectionName {
				ps = ro.Status.Parents[0]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).NotTo(BeNil())
			Expect(*ps.ParentRef.SectionName).To(Equal(gwapiv1.SectionName("gateway-1-listener-dtls")))
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

			// the new UDPRoute in the dummy-namespace
			ro = &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx,
				types.NamespacedName{Namespace: "dummy-namespace", Name: "dummy-namespace-route"},
				ro)).Should(Succeed())

			// 3rd listener accepts the route
			Expect(ro.Status.Parents).To(HaveLen(1))
			ps = ro.Status.Parents[0]
			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(testutils.TestNsName)))
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(BeNil())
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

		It("should be possible to remove the new route and the namespace", func() {
			// reset gw
			createOrUpdateGateway(ctx, k8sClient, &testutils.TestGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				})

				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:          gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:          gwapiv1.PortNumber(1),
					Protocol:      gwapiv1.ProtocolType("TURN-UDP"),
					AllowedRoutes: nil,
				}, {
					Name:          gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:          gwapiv1.PortNumber(2),
					Protocol:      gwapiv1.ProtocolType("TURN-DTLS"), // exposed even if mixed-proto-lb is disabled
					TLS:           &tls,
					AllowedRoutes: nil,
				}}
			})
			createOrUpdateUDPRoute(ctx, k8sClient, &testutils.TestUDPRoute, nil)
			Expect(k8sClient.Delete(ctx, &stnrgwv1.UDPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dummy-namespace-route",
					Namespace: "dummy-namespace",
				},
			})).Should(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dummy-namespace",
				},
			})).Should(Succeed())
		})
	})

	Context("The controller should dynamically render a new valid STUNner config (EDS DISABLED) when", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("changing the parentRef of a route", func() {
			ctrl.Log.Info("re-loading UDPRoute: ParentRef.SectionName = dummy")
			createOrUpdateUDPRoute(ctx, k8sClient, &testutils.TestUDPRoute, func(current *stnrgwv1.UDPRoute) {
				sn := gwapiv1.SectionName("dummy")
				current.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
				// stnrgwv1.ObjectName("dummy")
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) > 0 && len(c.Listeners) != 2 ||
					len(c.Clusters) == 0 && len(c.Listeners[0].Routes) == 0 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(0))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(0))

			Expect(conf.Clusters).To(HaveLen(0))

			ro := &stnrgwv1.UDPRoute{}
			// wait until status gets updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute), ro)
				if err != nil || ro == nil {
					return false
				}

				if len(ro.Status.Parents) != 1 {
					return false
				}

				if ro.Status.Parents[0].ParentRef.SectionName != nil &&
					*ro.Status.Parents[0].ParentRef.SectionName == gwapiv1.SectionName("dummy") {
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			// Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
			// 	ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).NotTo(BeNil())
			Expect(*ps.ParentRef.SectionName).To(Equal(gwapiv1.SectionName("dummy")))
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))

			// backendRefs resolved, to ResolvedRefs=True
			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

		It("changing the auth type", func() {
			atype := "ephemeral" // use alias -> ephemeral
			secret := "dummy"

			ctrl.Log.Info("re-loading original UDPRoute")
			createOrUpdateUDPRoute(ctx, k8sClient, &testutils.TestUDPRoute, func(current *stnrgwv1.UDPRoute) {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
			})

			ctrl.Log.Info("re-loading gateway-config")
			createOrUpdateGatewayConfig(ctx, k8sClient, &testutils.TestGwConfig, func(current *stnrgwv1.GatewayConfig) {
				current.Spec.AuthType = &atype
				current.Spec.SharedSecret = &secret
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if c.Auth.Type == "ephemeral" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Auth.Type).Should(Equal("ephemeral"))
			Expect(conf.Auth.Credentials["secret"]).Should(Equal("dummy"))
		})

		// external auth ref
		It("switching to external auth refs", func() {
			ctrl.Log.Info("loading the external auth Secret")
			createOrUpdateSecret(ctx, k8sClient, testAuthSecret, nil)

			ctrl.Log.Info("re-loading gateway-config")
			namespace := gwapiv1.Namespace("testnamespace")
			createOrUpdateGatewayConfig(ctx, k8sClient, &testutils.TestGwConfig, func(current *stnrgwv1.GatewayConfig) {
				atype := "timewindowed" // use alias -> ephemeral
				current.Spec.AuthType = &atype
				current.Spec.Username = nil
				current.Spec.Password = nil
				current.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if c.Auth.Type == "static" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Auth.Type).Should(Equal("static"))
			Expect(conf.Auth.Credentials["username"]).Should(Equal("ext-testuser"))
			Expect(conf.Auth.Credentials["password"]).Should(Equal("ext-testpass"))
		})

		It("external auth refs override inline auth", func() {
			ctrl.Log.Info("re-loading gateway-config")
			namespace := gwapiv1.Namespace("testnamespace")
			createOrUpdateGatewayConfig(ctx, k8sClient, &testutils.TestGwConfig, func(current *stnrgwv1.GatewayConfig) {
				atype := "ephemeral"
				current.Spec.AuthType = &atype
				current.Spec.Username = nil
				current.Spec.Password = nil
				sharedSecret := "testsecret"
				current.Spec.SharedSecret = &sharedSecret
				current.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if c.Auth.Type == "static" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Auth.Type).Should(Equal("static"))
			Expect(conf.Auth.Credentials["username"]).Should(Equal("ext-testuser"))
			Expect(conf.Auth.Credentials["password"]).Should(Equal("ext-testpass"))
		})

		It("updating the external auth ref should re-generate the config", func() {
			ctrl.Log.Info("re-loading the external auth Secret")
			createOrUpdateSecret(ctx, k8sClient, &testutils.TestAuthSecret, func(current *corev1.Secret) {
				current.Data["username"] = []byte("new-user")
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				u, ok := c.Auth.Credentials["username"]
				if ok && u == "new-user" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Auth.Type).Should(Equal("static"))
			Expect(conf.Auth.Credentials["username"]).Should(Equal("new-user"))
			Expect(conf.Auth.Credentials["password"]).Should(Equal("ext-testpass"))
		})

		It("cnanging the external auth ref type should re-generate the config", func() {
			ctrl.Log.Info("re-loading the external auth Secret")
			createOrUpdateSecret(ctx, k8sClient, &testutils.TestAuthSecret, func(current *corev1.Secret) {
				current.Data["type"] = []byte("ephemeral")
				current.Data["secret"] = []byte("dummy")
				delete(current.Data, "username")
				delete(current.Data, "password")
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if c.Auth.Type == "ephemeral" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Auth.Type).Should(Equal("ephemeral"))
			Expect(conf.Auth.Credentials["secret"]).Should(Equal("dummy"))
		})

		It("external auth refs with missing Secret should fail", func() {
			ctrl.Log.Info("deleting auth Secret")
			Expect(k8sClient.Delete(ctx, testAuthSecret)).Should(Succeed())

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				conf, ok := cm.Data[opdefault.DefaultStunnerdConfigfileName]

				if ok && conf == "" {
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())
		})

		It("fallback to inline auth defs", func() {
			ctrl.Log.Info("re-loading gateway-config with inline auth")
			createOrUpdateGatewayConfig(ctx, k8sClient, &testutils.TestGwConfig, func(current *stnrgwv1.GatewayConfig) {
				atype := "timewindowed" // use alias -> ephemeral
				secret := "dummy"
				current.Spec.AuthType = &atype
				current.Spec.SharedSecret = &secret
				current.Spec.AuthRef = nil
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if c.Auth.Type == "ephemeral" {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			Expect(conf.Auth.Type).Should(Equal("ephemeral"))
			Expect(conf.Auth.Credentials["secret"]).Should(Equal("dummy"))
		})

		It("changing a listener port", func() {
			// the client may overwrite our objects, recreate!

			ctrl.Log.Info("re-loading gateway")
			createOrUpdateGateway(ctx, k8sClient, &testutils.TestGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				})

				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:          gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:          gwapiv1.PortNumber(1234),
					Protocol:      gwapiv1.ProtocolType("TURN-UDP"),
					AllowedRoutes: nil,
				}, {
					Name:          gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:          gwapiv1.PortNumber(2),
					Protocol:      gwapiv1.ProtocolType("TURN-DTLS"), // exposed even if mixed-proto-lb is disabled
					TLS:           &tls,
					AllowedRoutes: nil,
				}}
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
			createOrUpdateUDPRoute(ctx, k8sClient, &testutils.TestUDPRoute, func(current *stnrgwv1.UDPRoute) {
				current.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gwapiv1.ObjectName("dummy")
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
			createOrUpdateUDPRoute(ctx, k8sClient, &testutils.TestUDPRoute, func(current *stnrgwv1.UDPRoute) {})

			ctrl.Log.Info("adding a new route")
			current := &stnrgwv1.UDPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "route-2",
				Namespace: testUDPRoute.GetNamespace(),
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
				current.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gwapiv1.ObjectName("dummy")
				current.Spec.Rules[0].BackendRefs = append(current.Spec.Rules[0].BackendRefs,
					stnrgwv1.BackendRef{
						BackendObjectReference: stnrgwv1.BackendObjectReference{
							Name: gwapiv1.ObjectName("dummy-2"),
							Port: &testutils.TestPort,
						},
					})

				// try to attach to all listeners: p1 -> udp, p2 -> dummy, p3 -> tcp
				sn := gwapiv1.SectionName("invalid")
				current.Spec.CommonRouteSpec.ParentRefs = append(current.Spec.CommonRouteSpec.ParentRefs,
					gwapiv1.ParentReference{
						Name:        gwapiv1.ObjectName("gateway-1"),
						SectionName: &sn,
					})

				sn2 := gwapiv1.SectionName("gateway-1-listener-dtls")
				current.Spec.CommonRouteSpec.ParentRefs = append(current.Spec.CommonRouteSpec.ParentRefs,
					gwapiv1.ParentReference{
						Name:        gwapiv1.ObjectName("gateway-1"),
						SectionName: &sn2,
					})

				return nil
			})
			Expect(err).Should(Succeed())

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) == 2 && len(c.Listeners[0].Routes) == 2 && len(c.Clusters) == 2 {
					conf = &c
					return true
				}

				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")

			Expect(conf.Listeners).To(HaveLen(2))
			l := conf.Listeners[0]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Routes).To(HaveLen(2))
			Expect(l.Routes).Should(ContainElement("testnamespace/udproute-ok"))
			Expect(l.Routes).Should(ContainElement("testnamespace/route-2"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-dtls" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes).Should(ContainElement("testnamespace/route-2"))

			Expect(conf.Clusters).To(HaveLen(2))

			c := conf.Clusters[0]
			if c.Name != "testnamespace/udproute-ok" {
				c = conf.Clusters[1]
			}

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints).Should(ContainElement("testservice-ok.testnamespace.svc.cluster.local"))

			c = conf.Clusters[1]
			if c.Name != "testnamespace/route-2" {
				c = conf.Clusters[0]
			}

			Expect(c.Name).Should(Equal("testnamespace/route-2"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(2))
			Expect(c.Endpoints).Should(ContainElement("dummy.testnamespace.svc.cluster.local"))
			Expect(c.Endpoints).Should(ContainElement("dummy-2.testnamespace.svc.cluster.local"))
		})

		It("adding a new gateway", func() {
			ctrl.Log.Info("updating route-2 to point to both the old and the new gateway")
			ro := &stnrgwv1.UDPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "route-2",
				Namespace: testUDPRoute.GetNamespace(),
			}}

			_, err := createOrUpdate(ctx, k8sClient, ro, func() error {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&ro.Spec)

				ro.Spec.Rules[0].BackendRefs[0].BackendObjectReference.Name =
					gwapiv1.ObjectName("dummy")
				ro.Spec.Rules[0].BackendRefs = append(ro.Spec.Rules[0].BackendRefs,
					stnrgwv1.BackendRef{
						BackendObjectReference: stnrgwv1.BackendObjectReference{
							Name: gwapiv1.ObjectName("dummy-2"),
							Port: &testutils.TestPort,
						},
					})

				// try to attach to all listeners: p1 -> udp, p2 -> dummy, p3 -> tcp
				sn := gwapiv1.SectionName("gateway-2-udp")
				ro.Spec.CommonRouteSpec.ParentRefs = append(ro.Spec.CommonRouteSpec.ParentRefs,
					gwapiv1.ParentReference{
						Name:        gwapiv1.ObjectName("gateway-2"),
						SectionName: &sn,
					})

				sn2 := gwapiv1.SectionName("gateway-2-dtls")
				ro.Spec.CommonRouteSpec.ParentRefs = append(ro.Spec.CommonRouteSpec.ParentRefs,
					gwapiv1.ParentReference{
						Name:        gwapiv1.ObjectName("gateway-2"),
						SectionName: &sn2,
					})

				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("re-loading the test gateway")
			createOrUpdateGateway(ctx, k8sClient, &testutils.TestGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				})

				mode := gwapiv1.TLSModeTerminate
				ns := gwapiv1.Namespace("testnamespace")
				tls := gwapiv1.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1.ObjectName("testsecret-ok"),
					}},
				}
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:          gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:          gwapiv1.PortNumber(1234),
					Protocol:      gwapiv1.ProtocolType("TURN-UDP"),
					AllowedRoutes: nil,
				}, {
					Name:          gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:          gwapiv1.PortNumber(2),
					Protocol:      gwapiv1.ProtocolType("TURN-DTLS"), // exposed even if mixed-proto-lb is disabled
					TLS:           &tls,
					AllowedRoutes: nil,
				}}
			})

			ctrl.Log.Info("adding a new gateway")
			current := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: testutils.TestGw.GetNamespace(),
			}}

			_, err = ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testutils.TestGw.Spec.DeepCopyInto(&current.Spec)

				current.Spec.Listeners[0].Name =
					gwapiv1.SectionName("gateway-2-udp")
				current.Spec.Listeners[0].Port =
					gwapiv1.PortNumber(1234)
				current.Spec.Listeners[1].Name =
					gwapiv1.SectionName("gateway-2-tcp")
				current.Spec.Listeners[1].Port =
					gwapiv1.PortNumber(4321)

				return nil
			})
			Expect(err).Should(Succeed())

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
			l := stnrconfv1.ListenerConfig{}

			for _, _l := range conf.Listeners {
				if _l.Name == "testnamespace/gateway-1/gateway-1-listener-udp" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Routes).To(HaveLen(2))
			Expect(l.Routes).Should(ContainElement("testnamespace/udproute-ok"))
			Expect(l.Routes).Should(ContainElement("testnamespace/route-2"))

			for _, _l := range conf.Listeners {
				if _l.Name == "testnamespace/gateway-1/gateway-1-listener-dtls" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Routes).To(HaveLen(0))

			for _, _l := range conf.Listeners {
				if _l.Name == "testnamespace/gateway-2/gateway-2-udp" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("testnamespace/gateway-2/gateway-2-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1234))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes).Should(ContainElement("testnamespace/route-2"))

			for _, _l := range conf.Listeners {
				if _l.Name == "testnamespace/gateway-2/gateway-2-tcp" {
					l = _l
					break
				}
			}
			Expect(l.Name).Should(Equal("testnamespace/gateway-2/gateway-2-tcp"))
			Expect(l.Protocol).Should(Equal("TURN-TCP"))
			Expect(l.Port).Should(Equal(4321))
			Expect(l.Routes).To(HaveLen(0))

			Expect(conf.Clusters).To(HaveLen(2))
			c := conf.Clusters[0]
			if c.Name != "testnamespace/udproute-ok" {
				c = conf.Clusters[1]
			}

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STRICT_DNS"))
			Expect(c.Endpoints).To(HaveLen(1))
			Expect(c.Endpoints).Should(ContainElement("testservice-ok.testnamespace.svc.cluster.local"))

			c = conf.Clusters[1]
			if c.Name != "testnamespace/route-2" {
				c = conf.Clusters[0]
			}

			Expect(c.Name).Should(Equal("testnamespace/route-2"))
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
			gw := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: testutils.TestGw.GetNamespace(),
			}}
			Expect(k8sClient.Delete(ctx, gw)).Should(Succeed())

			ctrl.Log.Info("deleting route-3")
			ro := &stnrgwv1.UDPRoute{ObjectMeta: metav1.ObjectMeta{
				Name:      "route-2",
				Namespace: testUDPRoute.GetNamespace(),
			}}
			Expect(k8sClient.Delete(ctx, ro)).Should(Succeed())

			// restore
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
		})

		It("should render an empty config", func() {
			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
	Context("When creating a minimal set of API resources (EDS ENABLED, RELAY-TO-CLUSTER-IP DISABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = false

			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)
			createOrUpdateGateway(ctx, k8sClient, &testutils.TestGw, nil)
			createOrUpdateUDPRoute(ctx, k8sClient, &testutils.TestUDPRoute, nil)
			createOrUpdateService(ctx, k8sClient, testSvc, nil)

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) == 2 && len(c.Listeners[0].Routes) == 1 &&
					len(c.Clusters) == 1 && len(c.Clusters[0].Endpoints) == 4 {
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
			Expect(c.Endpoints).To(HaveLen(4))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))
		})

		It("should set the Route status", func() {
			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			// Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(gwapiv1.Namespace("testnamespace"))))
			// Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			// Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController("gatewayclass-ok")))

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

			// restoure
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
		})
	})

	// WITH EDS, WITHOUT RELAY-CLUSTER-IP
	Context("When creating a minimal set of API resources (EDS ENABLED, RELAY-TO-CLUSTER-IP DISABLED, ENDPOINTSLICE-CONTROLLER-ENABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = false

			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, nil)
			createOrUpdateService(ctx, k8sClient, testSvc, nil)

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) == 2 && len(c.Listeners[0].Routes) == 1 &&
					len(c.Clusters) == 1 && len(c.Clusters[0].Endpoints) == 4 {
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
			Expect(c.Endpoints).To(HaveLen(4))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))
		})

		It("should set the Route status", func() {
			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			// Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(gwapiv1.Namespace("testnamespace"))))
			// Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			// Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController("gatewayclass-ok")))

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

			// restoure
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
		})
	})

	// WITH EDS and RELAY-CLUSTER-IP
	Context("When creating a minimal set of API resources (EDS ENABLED, RELAY-TO-CLUSTER-IP ENABLED, ENDPOINTSLICE-CONTROLLER-ENABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			ctrl.Log.Info("re-loading Gateway with 1 valid listener")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("invalid"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("dummy"),
				}}
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}
			cm := &corev1.ConfigMap{}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))

			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))

			svc := store.Services.GetObject(types.NamespacedName{
				Namespace: "testnamespace", Name: "testservice-ok"})
			Expect(c.Endpoints).Should(ContainElement(svc.Spec.ClusterIP))

			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
		})
	})

	Context("When changing a route parentref to the TCP listener (EDS ENABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}
		sn := gwapiv1.SectionName("gateway-1-listener-tcp")

		It("should render a valid STUNner config", func() {
			ctrl.Log.Info("re-loading Gateway with 2 valid listeners")
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)

			ctrl.Log.Info("re-loading UDPRoute")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, func(current *stnrgwv1.UDPRoute) {
				current.Spec.CommonRouteSpec.ParentRefs[0].SectionName = &sn
			})

			lookupKey := types.NamespacedName{
				Name:      opdefault.DefaultConfigMapName,
				Namespace: string(testutils.TestNsName),
			}

			ctrl.Log.Info("trying to Get STUNner configmap", "resource", lookupKey)
			Eventually(func() bool {
				cm := &corev1.ConfigMap{}
				err := k8sClient.Get(ctx, lookupKey, cm)
				if err != nil {
					return false
				}

				c, err := store.UnpackConfigMap(cm)
				if err != nil {
					return false
				}

				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				if len(c.Listeners[0].Routes) == 0 && len(c.Listeners[1].Routes) == 1 {
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
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(0))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TURN-TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))
			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
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
			// wait until gateway is programmed
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionFalse // there is an invalid listener: programmed->false

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

			// listeners[1]: detached
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

			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(sn)))
			Expect(ps.ControllerName).To(Equal(gwapiv1.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
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

			ctrl.Log.Info("deleting Node 1")
			Expect(k8sClient.Delete(ctx, testNode)).Should(Succeed())

			ctrl.Log.Info("deleting Node 2")
			node2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}}
			Expect(k8sClient.Delete(ctx, node2)).Should(Succeed())

			// restore
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
		})
	})
}
