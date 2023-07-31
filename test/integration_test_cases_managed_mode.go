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
	"time"
	// "reflect"
	// "testing"
	// "fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

func testManagedMode() {
	// SINGLE GATEWAY
	Context("When creating a minimal set of API resources", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// enable envtest-compatibility model
			config.EnvTestCompatibilityMode = true

			// switch EDS on
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			ctrl.Log.Info("loading GatewayClass")
			Expect(k8sClient.Create(ctx, testGwClass)).Should(Succeed())

			ctrl.Log.Info("loading GatewayConfig")
			Expect(k8sClient.Create(ctx, testGwConfig)).Should(Succeed())

			ctrl.Log.Info("loading Gateway")
			Expect(k8sClient.Create(ctx, testGw)).Should(Succeed())

			ctrl.Log.Info("loading default Dataplane")
			current := &stnrv1a1.Dataplane{ObjectMeta: metav1.ObjectMeta{
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
			gwConfig := &stnrv1a1.GatewayConfig{}
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
			lookupKey := store.GetNamespacedName(testGw)
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
			lookupKey := store.GetNamespacedName(testGw)
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
			lookupKey := store.GetNamespacedName(testGw)
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
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
		})

		It("should set the GatewayClass status", func() {
			gc := &gwapiv1a2.GatewayClass{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGwClass),
				gc)).Should(Succeed())

			Expect(gc.Status.Conditions).To(HaveLen(1))

			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gwapiv1b1.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gwapiv1b1.GatewayClassReasonAccepted)))
		})

		It("should set the Gateway status", func() {
			// wait until gateway is programmed
			gw := &gwapiv1a2.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1b1.GatewayConditionProgrammed))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}

				if len(gw.Status.Listeners) != 3 {
					return false
				}

				s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1b1.ListenerConditionReady))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}

				return true
			}, timeout, interval).Should(BeTrue())

			Expect(gw.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			// listeners: no public gateway address so Ready status is False
			Expect(gw.Status.Listeners).To(HaveLen(3))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[1]: detached
			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonUnsupportedProtocol)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[2]: ok
			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

		})

		It("should survive the event of adding a route", func() {
			ctrl.Log.Info("loading UDPRoute")
			// fmt.Printf("%#v\n", testUDPRoute)
			Expect(k8sClient.Create(ctx, testUDPRoute)).Should(Succeed())

			ctrl.Log.Info("loading backend Service/Endpoint")
			Expect(k8sClient.Create(ctx, testSvc)).Should(Succeed())
			Expect(k8sClient.Create(ctx, testEndpoint)).Should(Succeed())

			lookupKey := store.GetNamespacedName(testGw)
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
				if len(c.Clusters) == 1 {
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
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
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
			ro := &gwapiv1a2.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			// Expect(ps.ParentRef.Namespace).To(HaveValue(Equal(gwapiv1a2.Namespace("testnamespace"))))
			// Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-1")))
			// Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController("gatewayclass-ok")))

			// Expect(ps.ParentRef.Group).To(BeNil())
			// Expect(ps.ParentRef.Kind).To(BeNil())
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(testutils.TestSectionName)))
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

		It("should install a NodePort public IP/port", func() {
			ctrl.Log.Info("re-loading gateway")
			createOrUpdateGateway(&testutils.TestGw, nil)

			ctrl.Log.Info("loading a Kubernetes Node")
			createOrUpdateNode(&testutils.TestNode, nil)

			lookupKey := store.GetNamespacedName(testGw)
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

		It("should install TLS cert/keys", func() {
			ctrl.Log.Info("loading TLS Secret")
			createOrUpdateSecret(&testutils.TestSecret, nil)

			ctrl.Log.Info("re-loading gateway with TLS cert/key the 2nd listener")
			createOrUpdateGateway(&testutils.TestGw, func(current *gwapiv1a2.Gateway) {
				mode := gwapiv1b1.TLSModeTerminate
				ns := gwapiv1a2.Namespace("testnamespace")
				tls := gwapiv1a2.GatewayTLSConfig{
					Mode: &mode,
					CertificateRefs: []gwapiv1a2.SecretObjectReference{{
						Namespace: &ns,
						Name:      gwapiv1a2.ObjectName("testsecret-ok"),
					}},
				}

				current.Spec.Listeners[1].Name = gwapiv1a2.SectionName("gateway-1-listener-dtls")
				current.Spec.Listeners[1].Protocol = gwapiv1a2.ProtocolType("DTLS")
				current.Spec.Listeners[1].TLS = &tls
			})

			lookupKey := store.GetNamespacedName(testGw)
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

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[1].Cert != "" && c.Listeners[1].Key != "" {
					conf = &c
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(3))
			l := conf.Listeners[0]

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("DTLS"))
			Expect(l.Port).Should(Equal(3))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Cert).Should(Equal(testutils.TestCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())

			l = conf.Listeners[2]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should update TLS cert when Secret changes", func() {
			ctrl.Log.Info("re-loading TLS Secret")
			createOrUpdateSecret(&testutils.TestSecret, func(current *corev1.Secret) {
				current.Data["tls.crt"] = []byte("newcert")
			})

			lookupKey := store.GetNamespacedName(testGw)
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

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[1].Cert == newCert64 {
					conf = &c
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(3))
			l := conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("DTLS"))
			Expect(l.Port).Should(Equal(3))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Cert).Should(Equal(newCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should create a Deployment for the Gateway", func() {
			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, deploy)
				return err == nil && deploy != nil
			}, timeout, interval).Should(BeTrue())

			Expect(deploy).NotTo(BeNil(), "Deployment rendered")

			// metadata
			gwName, ok := deploy.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "Gateway label")
			Expect(gwName).Should(Equal(store.GetObjectKey(testGw)), "Gateway label value")

			labs := deploy.GetLabels()
			v, ok := labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			v, ok = labs["dummy-label"] // comes from testGw
			Expect(ok).Should(BeTrue(), "gw label")
			Expect(v).Should(Equal("dummy-value"))

			Expect(store.IsOwner(testGw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// spec

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Set{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue},
				labels.Set{opdefault.RelatedGatewayKey: testGw.GetName()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(2))
			v, ok = labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetName()))

			// deployment selector matches pod template
			Expect(selector.Matches(labels.Set(labs))).Should(BeTrue())

			// unit tests check the rest: only test the basics here
			podSpec := &podTemplate.Spec
			Expect(podSpec.Containers).To(HaveLen(1))
			container := podSpec.Containers[0]
			Expect(container.Name).Should(Equal(opdefault.DefaultStunnerdInstanceName))
			Expect(container.Image).Should(Equal("testimage-1"))
			Expect(container.Resources.Limits).Should(Equal(testutils.TestResourceLimit))
			Expect(container.Resources.Requests).Should(Equal(testutils.TestResourceRequest))

			// Expect(container.VolumeMounts).To(HaveLen(1))

			Expect(container.ImagePullPolicy).Should(Equal(corev1.PullAlways))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeTrue())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should update the Deployment when the Dataplane changes", func() {
			ctrl.Log.Info("adding the default Dataplane")
			current := &stnrv1a1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: testDataplane.GetName(),
			}}

			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testDataplane.Spec.DeepCopyInto(&current.Spec)

				current.Spec.Image = "dummy-image"
				current.Spec.Command[0] = "dummy-command"
				current.Spec.Args[1] = "dummy-arg"

				current.Spec.HostNetwork = false
				return nil
			})
			Expect(err).Should(Succeed())

			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, deploy)
				return err == nil && deploy != nil && deploy.Spec.Template.Spec.HostNetwork == false
			}, timeout, interval).Should(BeTrue())

			Expect(deploy).NotTo(BeNil(), "Deployment rendered")

			// metadata
			gwName, ok := deploy.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "Gateway label")
			Expect(gwName).Should(Equal(store.GetObjectKey(testGw)), "Gateway label value")

			labs := deploy.GetLabels()
			v, ok := labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			v, ok = labs["dummy-label"] // comes from testGw
			Expect(ok).Should(BeTrue(), "gw label")
			Expect(v).Should(Equal("dummy-value"))

			Expect(store.IsOwner(testGw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// spec

			podTemplate := &deploy.Spec.Template
			podSpec := &podTemplate.Spec

			Expect(podSpec.Containers).To(HaveLen(1))

			container := podSpec.Containers[0]
			Expect(container.Name).Should(Equal(opdefault.DefaultStunnerdInstanceName))
			Expect(container.Image).Should(Equal("dummy-image"))
			Expect(container.Command[0]).Should(Equal("dummy-command"))
			Expect(container.Args[1]).Should(Equal("dummy-arg"))

			Expect(container.Resources.Limits).Should(Equal(testutils.TestResourceLimit))
			Expect(container.Resources.Requests).Should(Equal(testutils.TestResourceRequest))
			// Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.ImagePullPolicy).Should(Equal(corev1.PullAlways))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeFalse())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should survive converting the route to a StaticService backend", func() {
			ctrl.Log.Info("adding static service")
			Expect(k8sClient.Create(ctx, testStaticSvc)).Should(Succeed())

			ctrl.Log.Info("reseting gateway")
			createOrUpdateGateway(&testutils.TestGw, nil)

			ctrl.Log.Info("updating Route")
			createOrUpdateUDPRoute(&testutils.TestUDPRoute, func(current *gwapiv1a2.UDPRoute) {
				group := gwapiv1a2.Group(stnrv1a1.GroupVersion.Group)
				kind := gwapiv1a2.Kind("StaticService")
				current.Spec.CommonRouteSpec = gwapiv1a2.CommonRouteSpec{
					ParentRefs: []gwapiv1a2.ParentReference{{
						Name: "gateway-1",
					}},
				}
				current.Spec.Rules[0].BackendRefs = []gwapiv1a2.BackendRef{{
					BackendObjectReference: gwapiv1a2.BackendObjectReference{
						Group: &group,
						Kind:  &kind,
						Name:  "teststaticservice-ok",
					},
				}}
			})

			// wait until configmap gets updated
			lookupKey := store.GetNamespacedName(testGw)
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
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
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
			gc := &gwapiv1a2.GatewayClass{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGwClass),
				gc)).Should(Succeed())

			Expect(gc.Status.Conditions).To(HaveLen(1))

			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gwapiv1b1.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gwapiv1b1.GatewayClassReasonAccepted)))

			// wait until gateway is programmed
			gw := &gwapiv1a2.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1b1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue

			}, timeout, interval).Should(BeTrue())

			Expect(gw.Status.Conditions).To(HaveLen(2))

			s = meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			// stragely recreating the gateway lets api-server to find the public ip
			// for the gw so Ready status becomes true (should investigate this)
			Expect(gw.Status.Listeners).To(HaveLen(3))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[1]: detached
			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonUnsupportedProtocol)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[2]: ok
			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			ro := &gwapiv1a2.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]
			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(BeNil())
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})
	})

	// MULTI-GATEWAY
	Context("When creating 2 Gateways", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("should survive loading all resources", func() {
			// switch EDS on
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			ctrl.Log.Info("loading Gateway 2")
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, gw2, func() error {
				testGw.Spec.DeepCopyInto(&gw2.Spec)
				gw2.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-2-listener-udp"),
					Port:     gwapiv1a2.PortNumber(10),
					Protocol: gwapiv1a2.ProtocolType("UDP"),
				}}
				return nil
			})
			Expect(err).Should(Succeed())

			// UDPRoute: both gateways are parents
			ctrl.Log.Info("updating UDPRoute")
			createOrUpdateUDPRoute(&testutils.TestUDPRoute, func(current *gwapiv1a2.UDPRoute) {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
				current.Spec.CommonRouteSpec = gwapiv1a2.CommonRouteSpec{
					ParentRefs: []gwapiv1a2.ParentReference{{
						Name:        "gateway-1",
						SectionName: &testutils.TestSectionName,
					}, {
						Name: "gateway-2",
					}},
				}
			})
		})

		It("should render a STUNner config for Gateway 1", func() {
			// retry, but also try to unpack inside Eventually
			lookupKey := store.GetNamespacedName(testGw)
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
				if len(c.Listeners) == 2 && len(c.Listeners[1].Routes) == 0 && len(c.Clusters) == 1 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			v, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			Expect(v).Should(Equal(store.GetObjectKey(testGw)))
			v, ok = cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			Expect(conf.Listeners).To(HaveLen(2))

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
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

		It("should render a STUNner config for Gateway 2", func() {
			// retry, but also try to unpack inside Eventually
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			cm := &corev1.ConfigMap{}
			lookupKey := store.GetNamespacedName(gw2)

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
				if len(c.Listeners) == 1 && len(c.Listeners[0].Routes) == 1 && len(c.Clusters) == 1 {
					conf = &c
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			v, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			Expect(v).Should(Equal(store.GetObjectKey(gw2)))
			v, ok = cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			Expect(conf.Listeners).To(HaveLen(1))

			l := conf.Listeners[0]
			Expect(l.Name).Should(Equal("testnamespace/gateway-2/gateway-2-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(10))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
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
		})

		It("should set the GatewayClass status", func() {
			gc := &gwapiv1a2.GatewayClass{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGwClass),
				gc)).Should(Succeed())

			Expect(gc.Status.Conditions).To(HaveLen(1))

			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gwapiv1b1.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gwapiv1b1.GatewayClassReasonAccepted)))
		})

		It("should set the status of Gateway 1", func() {
			gw := &gwapiv1a2.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1b1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			Expect(gw.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			// listeners: no public gateway address so Ready status is False
			Expect(gw.Status.Listeners).To(HaveLen(3))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[1]: detached
			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonUnsupportedProtocol)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[2]: ok
			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

		})

		It("should set the status of Gateway 2", func() {
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw2), gw2)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw2.Status.Conditions,
					string(gwapiv1b1.GatewayConditionProgrammed))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				// should get a public IP
				listenerStatuses := gw2.Status.Listeners
				if len(listenerStatuses) != 1 || listenerStatuses[0].Name != "gateway-2-listener-udp" {
					return false
				}

				s = meta.FindStatusCondition(listenerStatuses[0].Conditions,
					string(gwapiv1b1.GatewayConditionReady))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(gw2.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1b1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1b1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			// listeners: no public gateway address so Ready status is False
			Expect(gw2.Status.Listeners).To(HaveLen(1))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))
		})

		It("should set the Route status", func() {
			ro := &gwapiv1a2.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(2))

			ps := ro.Status.Parents[0]
			if ps.ParentRef.Name != gwapiv1a2.ObjectName("gateway-1") {
				ps = ro.Status.Parents[1]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(testutils.TestSectionName)))
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			ps = ro.Status.Parents[1]
			if ps.ParentRef.Name != gwapiv1a2.ObjectName("gateway-2") {
				ps = ro.Status.Parents[0]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-2")))
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

		It("should create a Deployment for Gateway 1", func() {
			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, deploy)
				return err == nil && deploy != nil
			}, timeout, interval).Should(BeTrue())

			Expect(deploy).NotTo(BeNil(), "Deployment rendered")

			// metadata
			gwName, ok := deploy.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "Gateway label")
			Expect(gwName).Should(Equal(store.GetObjectKey(testGw)), "Gateway label value")

			labs := deploy.GetLabels()
			v, ok := labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			v, ok = labs["dummy-label"] // comes from testGw
			Expect(ok).Should(BeTrue(), "gw label")
			Expect(v).Should(Equal("dummy-value"))

			Expect(store.IsOwner(testGw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Set{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue},
				labels.Set{opdefault.RelatedGatewayKey: testGw.GetName()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(2))
			v, ok = labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetName()))

			// deployment selector matches pod template
			Expect(selector.Matches(labels.Set(labs))).Should(BeTrue())

			// unit tests check the rest: only test the basics here
			podSpec := &podTemplate.Spec
			Expect(podSpec.Containers).To(HaveLen(1))
			container := podSpec.Containers[0]
			Expect(container.Name).Should(Equal(opdefault.DefaultStunnerdInstanceName))
			Expect(container.Image).Should(Equal("dummy-image"))
			Expect(container.Command[0]).Should(Equal("dummy-command"))
			Expect(container.Args[1]).Should(Equal("dummy-arg"))
			Expect(container.Resources.Limits).Should(Equal(testutils.TestResourceLimit))
			Expect(container.Resources.Requests).Should(Equal(testutils.TestResourceRequest))
			Expect(container.ImagePullPolicy).Should(Equal(corev1.PullAlways))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeFalse())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should create a Deployment for Gateway 2", func() {
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}

			ctrl.Log.Info("trying to Get Deployment", "resource", store.GetNamespacedName(deploy))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(gw2), deploy)
				return err == nil && deploy != nil
			}, timeout, interval).Should(BeTrue())

			Expect(deploy).NotTo(BeNil(), "Deployment rendered")

			// metadata
			gwName, ok := deploy.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "Gateway label")
			Expect(gwName).Should(Equal(store.GetObjectKey(gw2)), "Gateway label value")

			labs := deploy.GetLabels()
			v, ok := labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			_, ok = labs["dummy-label"] // testGw has it, hw2 doesn't
			Expect(ok).Should(BeFalse(), "gw label")

			// Get gw2 so that the owner-ref UID is filled in
			Expect(k8sClient.Get(ctx, store.GetNamespacedName(gw2), gw2)).Should(Succeed())
			Expect(store.IsOwner(gw2, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Set{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue},
				labels.Set{opdefault.RelatedGatewayKey: gw2.GetName()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(2))
			v, ok = labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetName()))

			// deployment selector matches pod template
			Expect(selector.Matches(labels.Set(labs))).Should(BeTrue())

			// unit tests check the rest: only test the basics here
			podSpec := &podTemplate.Spec
			Expect(podSpec.Containers).To(HaveLen(1))
			container := podSpec.Containers[0]
			Expect(container.Name).Should(Equal(opdefault.DefaultStunnerdInstanceName))
			Expect(container.Image).Should(Equal("dummy-image"))
			Expect(container.Resources.Limits).Should(Equal(testutils.TestResourceLimit))
			Expect(container.Resources.Requests).Should(Equal(testutils.TestResourceRequest))
			Expect(container.ImagePullPolicy).Should(Equal(corev1.PullAlways))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeFalse())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should survive deleting Gateway 2", func() {
			ctrl.Log.Info("deleting Gateway 2")
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Expect(k8sClient.Delete(ctx, gw2)).Should(Succeed())

			// wait until route status gets updated
			ro := &gwapiv1a2.UDPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(testUDPRoute), ro)
				if err != nil || ro == nil {
					return false
				}

				ps := ro.Status.Parents[0]
				if ps.ParentRef.Name != gwapiv1a2.ObjectName("gateway-2") {
					ps = ro.Status.Parents[1]
				}

				// fmt.Println("++++++++++++++++++++++")
				// fmt.Printf("%#v\n", ps)
				// fmt.Printf("%#v\n", ps.Conditions)
				// fmt.Println("++++++++++++++++++++++")

				s := meta.FindStatusCondition(ps.Conditions,
					string(gwapiv1a2.RouteConditionAccepted))
				if s != nil && s.Status == metav1.ConditionFalse {
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(2))

			ps := ro.Status.Parents[0]
			if ps.ParentRef.Name != gwapiv1a2.ObjectName("gateway-1") {
				ps = ro.Status.Parents[1]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(testutils.TestSectionName)))
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			ps = ro.Status.Parents[1]
			if ps.ParentRef.Name != gwapiv1a2.ObjectName("gateway-2") {
				ps = ro.Status.Parents[0]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-2")))
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

	})

	// MULTI-GATEWAYCLASS
	Context("When creating 2 GatewayClasses and Gateways", func() {
		conf := &stunnerconfv1alpha1.StunnerConfig{}

		It("should survive loading all resources", func() {
			// switch EDS on
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			ctrl.Log.Info("loading GatewayClass 2")
			gc2 := &gwapiv1a2.GatewayClass{ObjectMeta: metav1.ObjectMeta{
				Name: "gateway-class-2",
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, gc2, func() error {
				testGwClass.Spec.DeepCopyInto(&gc2.Spec)
				gc2.Spec.ParametersRef = &gwapiv1a2.ParametersReference{
					Group:     gwapiv1a2.Group(stnrv1a1.GroupVersion.Group),
					Kind:      gwapiv1a2.Kind("GatewayConfig"),
					Name:      "gateway-config-2",
					Namespace: &testutils.TestNsName,
				}

				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("loading GatewayConfig 2")
			realm := "testrealm-2"
			dataplane := "dataplane-2"
			gwConf2 := &stnrv1a1.GatewayConfig{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-config-2",
				Namespace: string(testutils.TestNsName),
			}}
			_, err = ctrlutil.CreateOrUpdate(ctx, k8sClient, gwConf2, func() error {
				testGwConfig.Spec.DeepCopyInto(&gwConf2.Spec)
				gwConf2.Spec.Realm = &realm
				gwConf2.Spec.Dataplane = &dataplane
				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("loading  Dataplane 2")
			dp2 := &stnrv1a1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: dataplane,
			}}
			pullPolicy := corev1.PullNever
			_, err = ctrlutil.CreateOrUpdate(ctx, k8sClient, dp2, func() error {
				testDataplane.Spec.DeepCopyInto(&dp2.Spec)
				dp2.Spec.Image = "testimage-2"
				dp2.Spec.Command = []string{"testcommand-2"}
				dp2.Spec.Args = []string{"arg-1", "arg-2"}
				dp2.Spec.ImagePullPolicy = &pullPolicy
				dp2.Spec.HostNetwork = true
				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("loading Gateway 2")
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			_, err = ctrlutil.CreateOrUpdate(ctx, k8sClient, gw2, func() error {
				testGw.Spec.DeepCopyInto(&gw2.Spec)
				gw2.Spec.GatewayClassName = gwapiv1a2.ObjectName("gateway-class-2")
				gw2.Spec.Listeners = []gwapiv1a2.Listener{{
					Name:     gwapiv1a2.SectionName("gateway-2-listener-udp"),
					Port:     gwapiv1a2.PortNumber(10),
					Protocol: gwapiv1a2.ProtocolType("UDP"),
				}}
				return nil
			})
			Expect(err).Should(Succeed())

			// UDPRoute: both gateways are parents
			ctrl.Log.Info("updating UDPRoute")
			createOrUpdateUDPRoute(&testutils.TestUDPRoute, func(current *gwapiv1a2.UDPRoute) {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
				current.Spec.CommonRouteSpec = gwapiv1a2.CommonRouteSpec{
					ParentRefs: []gwapiv1a2.ParentReference{{
						Name:        "gateway-1",
						SectionName: &testutils.TestSectionName,
					}, {
						Name: "gateway-2",
					}},
				}
			})
		})

		It("should render a STUNner config for Gateway 1", func() {
			// retry, but also try to unpack inside Eventually
			lookupKey := store.GetNamespacedName(testGw)
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
				if len(c.Listeners) == 2 && len(c.Listeners[1].Routes) == 0 && len(c.Clusters) == 1 {
					conf = &c
					return true
				}
				return false

			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			v, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			Expect(v).Should(Equal(store.GetObjectKey(testGw)))
			v, ok = cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			Expect(conf.Listeners).To(HaveLen(2))

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-tcp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
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

		It("should render a STUNner config for Gateway 2", func() {
			// retry, but also try to unpack inside Eventually
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			cm := &corev1.ConfigMap{}
			lookupKey := store.GetNamespacedName(gw2)

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
				if len(c.Listeners) == 1 && len(c.Clusters) == 1 {
					conf = &c
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
			v, ok := cm.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "GatewayConf namespace")
			Expect(v).Should(Equal(store.GetObjectKey(gw2)))
			v, ok = cm.GetLabels()[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			Expect(conf.Listeners).To(HaveLen(1))

			l := conf.Listeners[0]
			Expect(l.Name).Should(Equal("testnamespace/gateway-2/gateway-2-listener-udp"))
			Expect(l.Protocol).Should(Equal("UDP"))
			Expect(l.Port).Should(Equal(10))
			Expect(l.MinRelayPort).Should(Equal(1))
			Expect(l.MaxRelayPort).Should(Equal(2))
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
		})

		It("should set the status of GatewayClass 1", func() {
			gc := &gwapiv1a2.GatewayClass{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestGwClass),
				gc)).Should(Succeed())

			Expect(gc.Status.Conditions).To(HaveLen(1))

			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gwapiv1b1.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gwapiv1b1.GatewayClassReasonAccepted)))
		})

		It("should set the status of GatewayClass 2", func() {
			gc := &gwapiv1a2.GatewayClass{ObjectMeta: metav1.ObjectMeta{
				Name: "gateway-class-2",
			}}
			// wait until status gets updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gc), gc)
				if err != nil || gc == nil {
					return false
				}

				s := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1b1.GatewayClassConditionStatusAccepted))
				if s != nil && s.Status == metav1.ConditionTrue {
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(gc.Status.Conditions).To(HaveLen(1))
			s := meta.FindStatusCondition(gc.Status.Conditions,
				string(gwapiv1b1.GatewayClassConditionStatusAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayClassConditionStatusAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(
				Equal(string(gwapiv1b1.GatewayClassReasonAccepted)))
		})

		It("should set the status of Gateway 1", func() {
			gw := &gwapiv1a2.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testGw), gw)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw.Status.Conditions,
					string(gwapiv1b1.GatewayConditionProgrammed))
				return s.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			Expect(gw.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw.Status.Conditions,
				string(gwapiv1b1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			// listeners: no public gateway address so Ready status is False
			Expect(gw.Status.Listeners).To(HaveLen(3))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[1]: detached
			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonUnsupportedProtocol)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[1].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

			// listeners[2]: ok
			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw.Status.Listeners[2].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))

		})

		It("should set the status of Gateway 2", func() {
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw2), gw2)
				if err != nil {
					return false
				}

				// should be programmed
				s := meta.FindStatusCondition(gw2.Status.Conditions,
					string(gwapiv1b1.GatewayConditionProgrammed))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				// should get a public IP
				listenerStatuses := gw2.Status.Listeners
				if len(listenerStatuses) != 1 || listenerStatuses[0].Name != "gateway-2-listener-udp" {
					return false
				}

				s = meta.FindStatusCondition(listenerStatuses[0].Conditions,
					string(gwapiv1b1.GatewayConditionReady))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(gw2.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1b1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1b1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			Expect(gw2.Status.Listeners).To(HaveLen(1))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1b1.ListenerConditionReady))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1b1.ListenerConditionReady)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1b1.ListenerReasonReady)))
		})

		It("should set the Route status", func() {
			ro := &gwapiv1a2.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(2))

			ps := ro.Status.Parents[0]
			if ps.ParentRef.Name != gwapiv1a2.ObjectName("gateway-1") {
				ps = ro.Status.Parents[1]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-1")))
			Expect(ps.ParentRef.SectionName).To(HaveValue(Equal(testutils.TestSectionName)))
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s := meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			ps = ro.Status.Parents[1]
			if ps.ParentRef.Name != gwapiv1a2.ObjectName("gateway-2") {
				ps = ro.Status.Parents[0]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1a2.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1a2.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1a2.ObjectName("gateway-2")))
			Expect(ps.ControllerName).To(Equal(gwapiv1a2.GatewayController(config.ControllerName)))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1a2.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1a2.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
		})

		It("should create a Deployment for Gateway 1", func() {
			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, deploy)
				return err == nil && deploy != nil
			}, timeout, interval).Should(BeTrue())

			Expect(deploy).NotTo(BeNil(), "Deployment rendered")

			// metadata
			gwName, ok := deploy.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "Gateway label")
			Expect(gwName).Should(Equal(store.GetObjectKey(testGw)), "Gateway label value")

			labs := deploy.GetLabels()
			v, ok := labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			v, ok = labs["dummy-label"] // comes from testGw
			Expect(ok).Should(BeTrue(), "gw label")
			Expect(v).Should(Equal("dummy-value"))

			Expect(store.IsOwner(testGw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Set{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue},
				labels.Set{opdefault.RelatedGatewayKey: testGw.GetName()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(2))
			v, ok = labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetName()))

			// deployment selector matches pod template
			Expect(selector.Matches(labels.Set(labs))).Should(BeTrue())

			// unit tests check the rest: only test the basics here
			podSpec := &podTemplate.Spec
			Expect(podSpec.Containers).To(HaveLen(1))
			container := podSpec.Containers[0]
			Expect(container.Name).Should(Equal(opdefault.DefaultStunnerdInstanceName))
			Expect(container.Image).Should(Equal("dummy-image"))
			Expect(container.Command[0]).Should(Equal("dummy-command"))
			Expect(container.Args[1]).Should(Equal("dummy-arg"))
			Expect(container.Resources.Limits).Should(Equal(testutils.TestResourceLimit))
			Expect(container.Resources.Requests).Should(Equal(testutils.TestResourceRequest))
			Expect(container.ImagePullPolicy).Should(Equal(corev1.PullAlways))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeFalse())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should create a Deployment for Gateway 2", func() {
			gw2 := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}

			ctrl.Log.Info("trying to Get Deployment", "resource", store.GetNamespacedName(deploy))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(gw2), deploy)
				return err == nil && deploy != nil
			}, timeout, interval).Should(BeTrue())

			Expect(deploy).NotTo(BeNil(), "Deployment rendered")

			// metadata
			gwName, ok := deploy.GetAnnotations()[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "Gateway label")
			Expect(gwName).Should(Equal(store.GetObjectKey(gw2)), "Gateway label value")

			labs := deploy.GetLabels()
			v, ok := labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))

			_, ok = labs["dummy-label"] // testGw has it, hw2 doesn't
			Expect(ok).Should(BeFalse(), "gw label")

			// Get gw2 so that the owner-ref UID is filled in
			Expect(k8sClient.Get(ctx, store.GetNamespacedName(gw2), gw2)).Should(Succeed())
			Expect(store.IsOwner(gw2, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Set{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue},
				labels.Set{opdefault.RelatedGatewayKey: gw2.GetName()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(2))
			v, ok = labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetName()))

			// deployment selector matches pod template
			Expect(selector.Matches(labels.Set(labs))).Should(BeTrue())

			// unit tests check the rest: only test the basics here
			podSpec := &podTemplate.Spec
			Expect(podSpec.Containers).To(HaveLen(1))
			container := podSpec.Containers[0]
			Expect(container.Name).Should(Equal(opdefault.DefaultStunnerdInstanceName))
			Expect(container.Image).Should(Equal("testimage-2"))
			Expect(container.Command).Should(Equal([]string{"testcommand-2"}))
			Expect(container.Args).Should(Equal([]string{"arg-1", "arg-2"}))
			Expect(container.Resources.Limits).Should(Equal(testutils.TestResourceLimit))
			Expect(container.Resources.Requests).Should(Equal(testutils.TestResourceRequest))
			Expect(container.ImagePullPolicy).Should(Equal(corev1.PullNever))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeTrue())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should update only Gateway 2 when Dataplane 2 changes", func() {
			ctrl.Log.Info("updating  Dataplane 2")
			dp2 := &stnrv1a1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: "dataplane-2",
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, dp2, func() error {
				testDataplane.Spec.DeepCopyInto(&dp2.Spec)
				dp2.Spec.Image = "testimage-3"
				return nil
			})
			Expect(err).Should(Succeed())

			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-1",
				Namespace: string(testutils.TestNsName),
			}}
			ctrl.Log.Info("trying to Get Deployment for Gateway 1", "resource", store.GetNamespacedName(deploy))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(deploy), deploy)
				if err != nil || deploy == nil {
					return false
				}

				podSpec := &deploy.Spec.Template.Spec
				if len(podSpec.Containers) == 0 || podSpec.Containers[0].Image != "dummy-image" {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			deploy = &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			ctrl.Log.Info("trying to Get Deployment for Gateway 2", "resource", store.GetNamespacedName(deploy))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(deploy), deploy)
				if err != nil || deploy == nil {
					return false
				}

				podSpec := &deploy.Spec.Template.Spec
				if len(podSpec.Containers) == 0 || podSpec.Containers[0].Image != "testimage-3" {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
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

			// ctrl.Log.Info("deleting Service")
			// Expect(k8sClient.Delete(ctx, testSvc)).Should(Succeed())

			ctrl.Log.Info("deleting Endpoints")
			Expect(k8sClient.Delete(ctx, testEndpoint)).Should(Succeed())

			ctrl.Log.Info("deleting Dataplane")
			Expect(k8sClient.Delete(ctx, testDataplane)).Should(Succeed())

			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			config.EnvTestCompatibilityMode = false

			// Make sure all reconcile and update events are processed before channels
			// are closed (which would lead to a panic)
			time.Sleep(1 * time.Second)
		})
	})
}
