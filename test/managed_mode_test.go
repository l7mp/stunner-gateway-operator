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
	"fmt"
	"reflect"
	"time"

	// "reflect"
	// "testing"
	// "fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	cdsclient "github.com/l7mp/stunner/pkg/config/client"
	"github.com/l7mp/stunner/pkg/logger"
)

func testManagedMode() {
	// SINGLE GATEWAY
	Context("When creating a minimal set of API resources", Ordered, Label("managed"), func() {
		var conf *stnrv1.StunnerConfig
		var clientCtx context.Context
		var clientCancel context.CancelFunc
		var ch chan *stnrv1.StunnerConfig
		var cdsClient cdsclient.Client
		var licenseClient cdsclient.LicenseStatusClient

		BeforeAll(func() {
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch = make(chan *stnrv1.StunnerConfig, 128)
			var err error
			log := logger.NewLoggerFactory(stunnerLogLevel)
			cdsClient, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", "", log)
			Expect(err).Should(Succeed())
			Expect(cdsClient.Watch(clientCtx, ch, false)).Should(Succeed())
			licenseClient, err = cdsclient.NewLicenseStatusClient(cdsServerAddr, log.NewLogger("license-status"))
			Expect(err).Should(Succeed())
		})

		AfterAll(func() {
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			clientCancel()
			close(ch)
		})

		It("should survive loading a minimal config", func() {
			// switch EDS on
			createOrUpdateGatewayClass(ctx, k8sClient, testGwClass, nil)
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)

			ctrl.Log.Info("loading default Dataplane")
			current := &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: testDataplane.GetName(),
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testDataplane.Spec.DeepCopyInto(&current.Spec)
				return nil
			})
			Expect(err).Should(Succeed())

			// add a deamonset with the gateway's name: this allows to check later
			// whether lingering dataplane resources are properly cleaned up
			Expect(k8sClient.Create(ctx, testDaemonSet)).Should(Succeed())
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

		It("should return a default licensing status", func() {
			Eventually(func() bool {
				status, err := licenseClient.LicenseStatus(ctx)
				return err == nil && reflect.DeepEqual(status, stnrv1.NewEmptyLicenseStatus())
			}, timeout, interval).Should(BeTrue())
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
				if s == nil || s.Status != metav1.ConditionFalse { // should not be programmed: tcp listener has no public ip
					return false
				}

				if len(gw.Status.Listeners) != 2 {
					return false
				}

				s = meta.FindStatusCondition(gw.Status.Listeners[0].Conditions,
					string(gwapiv1.ListenerConditionAccepted))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				return true
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

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				if len(c.Clusters) == 1 && len(c.Clusters[0].Endpoints) == 5 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())
		})

		It("should render STUNner config with the new cluster", func() {
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

		It("should install a NodePort public IP/port", func() {
			ctrl.Log.Info("re-loading gateway")
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)

			ctrl.Log.Info("loading a Kubernetes Node")
			createOrUpdateNode(ctx, k8sClient, testNode, nil)

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// fmt.Printf("--------------------\nCHECKER 0: %#v\n--------------------\n", c)
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.4" {
					conf = c
					return true
				}

				return false
			}), timeout, interval).Should(BeTrue())
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

			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.5" && c.Listeners[0].PublicPort == 1 {
					conf = c
					return true
				}

				return false
			}), timeout, interval).Should(BeTrue())
			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
		})

		// we cannot set the public IP: no load-balancer operator in the envTest API server
		// -> check the nodeport fallback

		It("should install a NodePort public IP/port", func() {
			ctrl.Log.Info("re-loading gateway")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {})

			ctrl.Log.Info("loading a Kubernetes Node")
			createOrUpdateNode(ctx, k8sClient, testNode, nil)

			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.4" {
					conf = c
					return true
				}

				return false
			}), timeout, interval).Should(BeTrue())
		})

		It("should update the public IP/port when node External IP changes", func() {
			ctrl.Log.Info("re-loading Node with new External IP")
			statusUpdateNode(ctx, k8sClient, "testnode-ok", func(current *corev1.Node) {
				current.Status.Addresses[1].Address = "4.3.2.1"
			})

			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "4.3.2.1" {
					conf = c
					return true
				}

				return false

			}), timeout, interval).Should(BeTrue())
		})

		It("should remove the public IP/port when the node's External IP disappears", func() {
			ctrl.Log.Info("re-loading Node with no External IP")
			statusUpdateNode(ctx, k8sClient, "testnode-ok", func(current *corev1.Node) {
				current.Status.Addresses[1].Type = corev1.NodeInternalIP
			})

			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "" {
					conf = c
					return true
				}

				return false

			}), timeout, interval).Should(BeTrue())
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

			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.4" {
					conf = c
					return true
				}

				return false

			}), timeout, interval).Should(BeTrue())
		})

		It("should remove the public IP/port when the exposed LoadBalancer service type changes to ClusterIP", func() {
			ctrl.Log.Info("re-loading gateway-config with annotation: service-type: ClusterIP")
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, func(current *stnrgwv1.GatewayConfig) {
				current.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				current.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
				current.Spec.LoadBalancerServiceAnnotations[opdefault.ExternalTrafficPolicyAnnotationKey] =
					opdefault.ExternalTrafficPolicyAnnotationValue
			})

			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				if c.Listeners[0].PublicAddr == "" {
					conf = c
					return true
				}

				return false
			}), timeout, interval).Should(BeTrue())

		})

		It("should reset spec.ExternalTrafficPolicy=local for the ClusterIP service", func() {
			// spec.ExternalTrafficPolicy cannot be set for cluterip services
			svc := &corev1.Service{}
			Eventually(func() bool {
				lookupKey := store.GetNamespacedName(testGw)
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				return svc.Spec.ExternalTrafficPolicy !=
					corev1.ServiceExternalTrafficPolicyLocal
			}, timeout, interval).Should(BeTrue())

			Expect(svc.Spec.ExternalTrafficPolicy).Should(Equal(corev1.ServiceExternalTrafficPolicyType("")))
		})

		It("should handle session-affinity", func() {
			svc := &corev1.Service{}

			// default is SessionAffinity=clientIP
			Eventually(func() bool {
				lookupKey := store.GetNamespacedName(testGw)
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				return svc.Spec.SessionAffinity == corev1.ServiceAffinityClientIP
			}, timeout, interval).Should(BeTrue())
			Expect(svc.Spec.SessionAffinity).Should(Equal(corev1.ServiceAffinityClientIP))

			// disable session-affinity
			ctrl.Log.Info("re-loading gateway with annotation: stunner.l7mp.io/disable-session-affinity=true")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.DisableSessionAffiffinityAnnotationKey: // newline
					opdefault.DisableSessionAffiffinityAnnotationValue,
				})
			})

			Eventually(func() bool {
				lookupKey := store.GetNamespacedName(testGw)
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}

				return svc.Spec.SessionAffinity == corev1.ServiceAffinityNone ||
					string(svc.Spec.SessionAffinity) == ""
			}, timeout, interval).Should(BeTrue())
		})

		It("should restore the public IP/port when the exposed LoadBalancer service type changes to NodePort", func() {
			ctrl.Log.Info("re-loading gateway-config with annotation: service-type: ClusterIP")
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, func(current *stnrgwv1.GatewayConfig) {
				current.Spec.LoadBalancerServiceAnnotations = map[string]string{
					opdefault.ServiceTypeAnnotationKey:           "dummy",
					opdefault.ExternalTrafficPolicyAnnotationKey: opdefault.ExternalTrafficPolicyAnnotationValue,
				}
			})

			ctrl.Log.Info("re-loading gateway with annotation: service-type: NodePort")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				})
			})

			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 2 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[0].PublicAddr == "1.2.3.4" {
					conf = c
					return true
				}

				return false

			}), timeout, interval).Should(BeTrue())
		})

		It("should restore spec.ExternalTrafficPolicy=local for the NodePort service", func() {
			Eventually(func() bool {
				lookupKey := store.GetNamespacedName(testGw)
				svc := &corev1.Service{}
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				return svc.Spec.ExternalTrafficPolicy ==
					corev1.ServiceExternalTrafficPolicyLocal
			}, timeout, interval).Should(BeTrue())
		})

		It("should add annotations from Gateway", func() {
			ctrl.Log.Info("re-loading gateway-config with annotation: service-type: ClusterIP")
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, func(current *stnrgwv1.GatewayConfig) {
				current.Spec.LoadBalancerServiceAnnotations = map[string]string{
					opdefault.ServiceTypeAnnotationKey:           "dummy",
					opdefault.ExternalTrafficPolicyAnnotationKey: "dummy",
				}
			})

			ctrl.Log.Info("re-loading gateway with further annotations")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey:           "NodePort",
					opdefault.ExternalTrafficPolicyAnnotationKey: opdefault.ExternalTrafficPolicyAnnotationValue,
					"someAnnotation":                             "dummy-1",
					"someOtherAnnotation":                        "dummy-2",
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

				if svc.Spec.ExternalTrafficPolicy !=
					corev1.ServiceExternalTrafficPolicyLocal {
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

			Eventually(func() bool {
				lookupKey := store.GetNamespacedName(testGw)
				svc := &corev1.Service{}
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				return svc.Spec.ExternalTrafficPolicy ==
					corev1.ServiceExternalTrafficPolicyLocal
			}, timeout, interval).Should(BeTrue())
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
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, func(current *stnrgwv1.GatewayConfig) {
				current.Spec.LoadBalancerServiceAnnotations = map[string]string{}
			})

			lookupKey := store.GetNamespacedName(testGw)

			// learn nodeports
			svc := &corev1.Service{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				return len(svc.Spec.Ports) == 1
			}, timeout, interval).Should(BeTrue())
			Expect(len(svc.Spec.Ports) == 1).To(BeTrue())
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
				// must change the spec othwewise the controller will not be
				// triggered
				infra := gwapiv1.GatewayInfrastructure{
					Labels: map[gwapiv1.LabelKey]gwapiv1.LabelValue{
						"dummy-label-key": "dummy-label-value",
					},
				}
				current.Spec.Infrastructure = &infra
			})

			// retry, but also check if a public address has been added
			Eventually(func() bool {
				svc = &corev1.Service{}
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}

				if len(svc.Spec.Ports) != 1 {
					return false
				}

				if svc.Spec.ExternalTrafficPolicy ==
					corev1.ServiceExternalTrafficPolicyLocal {
					return false
				}

				as := svc.GetAnnotations()
				a1, ok1 := as[opdefault.ServiceTypeAnnotationKey]
				a2, ok2 := as["someAnnotation"]
				a3, ok3 := as["someOtherAnnotation"]

				return ok1 && ok2 && ok3 && a1 == "NodePort" && a2 == "new-dummy-1" && a3 == "dummy-2"
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

		It("should stabilize", func() {
			op.Stabilize()
		})

		var nodeport int32

		It("should use a specific NodePort when requested in a Gateway annotation", func() {
			lookupKey := store.GetNamespacedName(testGw)

			// learn nodeport
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("dummy"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}}
			})

			svc := &corev1.Service{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				return len(svc.Spec.Ports) == 2
			}, timeout, interval).Should(BeTrue())

			nodeport = svc.Spec.Ports[1].NodePort
			switch nodeport {
			case 32767:
				nodeport = 32766 // max nodeport
			default:
				nodeport += 1
			}

			// set nodeport
			ann := fmt.Sprintf("{\"gateway-1-listener-udp\": %d}", nodeport)
			ctrl.Log.Info("re-loading gateway with the nodeport annotation")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.ServiceTypeAnnotationKey: "NodePort",
					opdefault.NodePortAnnotationKey:    ann,
				})
				// must apply again, otherwise we fall back to testGw listeners
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("dummy"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}}
			})

			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				return len(svc.Spec.Ports) == 2 && svc.Spec.Ports[1].NodePort == nodeport
			}, timeout, interval).Should(BeTrue())
		})

		It("should use also use a specific target port when requested in a Gateway annotation", func() {
			// set target port
			targetPort := 12345
			ann := fmt.Sprintf("{\"gateway-1-listener-udp\": %d, \"random-listener\": 321}", targetPort)
			ctrl.Log.Info("re-loading gateway with the nodeport annotation")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{
					opdefault.TargetPortAnnotationKey: ann,
				})
				current.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-random-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("dummy"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("dummy"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}}
			})

			lookupKey := store.GetNamespacedName(testGw)
			svc := &corev1.Service{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, svc); err != nil {
					return false
				}
				// the dummy listener is not added
				return len(svc.Spec.Ports) == 2 &&
					svc.Spec.Ports[1].TargetPort == intstr.FromInt(targetPort)
			}, timeout, interval).Should(BeTrue())

			// to be on the safe side
			Expect(svc.Spec.Ports[1].Port).Should(Equal(int32(3)))
			Expect(svc.Spec.Ports[1].TargetPort).Should(Equal(intstr.FromInt(targetPort)))

			// get the config: the targetport should be enforced!
			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) == 2 && c.Listeners[1].Name == "testnamespace/gateway-1/gateway-1-listener-udp" &&
					c.Listeners[1].Port == targetPort {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())
			Expect(conf).NotTo(BeNil(), "STUNner config rendered")

			Expect(conf.Listeners).To(HaveLen(2))

			// not sure about the order
			l := conf.Listeners[0]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-random-udp" {
				l = conf.Listeners[1]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-random-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))

			l = conf.Listeners[1]
			if l.Name != "testnamespace/gateway-1/gateway-1-listener-udp" {
				l = conf.Listeners[0]
			}

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(targetPort))
		})

		It("should install TLS cert/keys", func() {
			ctrl.Log.Info("loading TLS Secret")
			createOrUpdateSecret(ctx, k8sClient, testSecret, nil)

			ctrl.Log.Info("re-loading gateway with TLS cert/key the 2nd listener")
			createOrUpdateGateway(ctx, k8sClient, testGw, func(current *gwapiv1.Gateway) {
				current.SetAnnotations(map[string]string{})
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
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-dtls"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("TURN-DTLS"),
					TLS:      &tls,
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-tcp"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-TCP"),
				}}
			})

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 3 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[1].Cert != "" && c.Listeners[1].Key != "" {
					conf = c
					return true
				}

				return false
			}), timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(3))
			l := conf.Listeners[0]

			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(1))
			Expect(l.Routes).To(HaveLen(1))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			l = conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(3))
			Expect(l.Cert).Should(Equal(testutils.TestCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())

			l = conf.Listeners[2]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-tcp"))
			Expect(l.Protocol).Should(Equal("TURN-TCP"))
			Expect(l.Port).Should(Equal(2))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should update TLS cert when Secret changes", func() {
			ctrl.Log.Info("re-loading TLS Secret")
			createOrUpdateSecret(ctx, k8sClient, testSecret, func(current *corev1.Secret) {
				current.Data["tls.crt"] = []byte("newcert")
			})

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) != 3 || len(c.Clusters) != 1 {
					return false
				}

				// the UDP listener should have a valid public IP set on both listeners
				if c.Listeners[1].Cert == newCert64 {
					conf = c
					return true
				}

				return false
			}), timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(3))
			l := conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(3))
			Expect(l.Cert).Should(Equal(newCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should create a Deployment for the Gateway", func() {
			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}
			gw := &gwapiv1.Gateway{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, deploy); err != nil || deploy == nil {
					return false
				}
				// must load the gw otherwise deploy.owner-ref will have invalid uid
				if err := k8sClient.Get(ctx, lookupKey, gw); err != nil || gw == nil {
					return false
				}
				return store.IsOwner(gw, deploy, "Gateway")
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

			Expect(store.IsOwner(gw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// spec

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Merge(
					labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
					labels.Set{opdefault.RelatedGatewayKey: testGw.GetName()},
				),
				labels.Set{opdefault.RelatedGatewayNamespace: testGw.GetNamespace()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(3))
			v, ok = labs[opdefault.AppLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.AppLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetName()))
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetNamespace()))

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
			// we always declare the metrics port, even if the metrics server is diabled
			Expect(container.Ports).To(HaveLen(1))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeTrue())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should clean up lingering dataplane resources", func() {
			lookupKey := store.GetNamespacedName(testGw)
			ds := &appv1.DaemonSet{}

			ctrl.Log.Info("trying to Get dataplane DaemonSet", "resource", lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, ds)
				return err != nil && apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should update config when the Dataplane changes", func() {
			ctrl.Log.Info("adding the default Dataplane")
			current := &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: testDataplane.GetName(),
			}}

			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testDataplane.Spec.DeepCopyInto(&current.Spec)

				current.Spec.Image = "dummy-image"
				current.Spec.Command[0] = "dummy-command"
				current.Spec.Args[1] = "dummy-arg"

				current.Spec.HostNetwork = false
				current.Spec.DisableHealthCheck = true
				current.Spec.EnableMetricsEnpoint = true

				current.Spec.Labels = map[string]string{"dummy-label-key": "dummy-value",
					"app": "conflicted-value"}
				current.Spec.Annotations = map[string]string{"dummy-annotation-key": "dummy-value",
					opdefault.RelatedGatewayKey: "conflicted-value"}

				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				if c.Admin.MetricsEndpoint != "" &&
					(c.Admin.HealthCheckEndpoint == nil || *c.Admin.HealthCheckEndpoint == "") {
					conf = c
					return true
				}

				return false
			}), timeout, interval).Should(BeTrue())

			Expect(conf.Admin.MetricsEndpoint).To(Equal(opdefault.DefaultMetricsEndpoint))
			Expect(conf.Admin.HealthCheckEndpoint == nil || *conf.Admin.HealthCheckEndpoint == "").To(BeTrue())

			Expect(conf.Listeners).To(HaveLen(3))
			l := conf.Listeners[1]
			Expect(l.Name).Should(Equal("testnamespace/gateway-1/gateway-1-listener-dtls"))
			Expect(l.Protocol).Should(Equal("TURN-DTLS"))
			Expect(l.Port).Should(Equal(3))
			Expect(l.Cert).Should(Equal(newCert64))
			Expect(l.Key).Should(Equal(testutils.TestKey64))
			Expect(l.Routes).Should(BeEmpty())
		})

		It("should update the Deployment after the Dataplane changed", func() {
			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}
			gw := &gwapiv1.Gateway{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, deploy); err != nil || deploy == nil ||
					deploy.Spec.Template.Spec.HostNetwork {
					return false
				}
				if err := k8sClient.Get(ctx, lookupKey, gw); err != nil || gw == nil {
					return false
				}
				return store.IsOwner(gw, deploy, "Gateway")
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

			Expect(store.IsOwner(gw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// spec

			podTemplate := &deploy.Spec.Template
			podSpec := &podTemplate.Spec

			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(4))
			v, ok = labs[opdefault.AppLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.AppLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetName()))
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetNamespace()))
			v, ok = labs["dummy-label-key"]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal("dummy-value"))

			as := podTemplate.GetAnnotations()
			Expect(as).To(HaveLen(2))
			v, ok = as[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "related-gw ann")
			Expect(v).Should(Equal(fmt.Sprintf("%s/%s",
				testGw.GetNamespace(), testGw.GetName())))
			v, ok = as["dummy-annotation-key"]
			Expect(ok).Should(BeTrue(), "dummy ann key")
			Expect(v).Should(Equal("dummy-value"))

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
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].Name).Should(Equal(opdefault.DefaultMetricsPortName))
			Expect(container.Ports[0].ContainerPort).Should(Equal(int32(stnrv1.DefaultMetricsPort)))
			Expect(container.Ports[0].Protocol).Should(Equal(corev1.ProtocolTCP))

			// remainder
			Expect(podSpec.TerminationGracePeriodSeconds).NotTo(BeNil())
			Expect(*podSpec.TerminationGracePeriodSeconds).Should(Equal(testutils.TestTerminationGrace))
			Expect(podSpec.HostNetwork).Should(BeFalse())
			Expect(podSpec.Affinity).To(BeNil())
		})

		It("should stabilize", func() {
			op.Stabilize()
		})

		It("should not reset the replica count when it is updated externally", func() {
			lookupKey := store.GetNamespacedName(testGw)

			replicas := int32(1)
			ctrl.Log.Info("updating the replica count in the Dataplane template",
				"resource", testDataplane.GetName(), "replica-count", replicas)
			dp := &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: testDataplane.GetName(),
			}}
			_, err := createOrUpdate(ctx, k8sClient, dp, func() error {
				dp.Spec.Replicas = &replicas
				// reset labels and anns for later tests
				dp.Spec.Labels = map[string]string{}
				dp.Spec.Annotations = map[string]string{}
				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("updating the replica count in the Deployment", "resource", lookupKey)
			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      testGw.GetName(),
				Namespace: testGw.GetNamespace(),
			}}
			var generation int64
			replicas = int32(4)
			_, err = createOrUpdate(ctx, k8sClient, deploy, func() error {
				deploy.Spec.Replicas = &replicas
				generation = deploy.GetGeneration()
				return nil
			})
			Expect(err).Should(Succeed())
			// we should have obtained a valid generation
			Expect(generation).NotTo(Equal(int64(0)))

			ctrl.Log.Info("waiting for the Deployment to be updated")
			time.Sleep(100 * time.Millisecond)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, deploy)
				return err == nil && deploy != nil && deploy.Spec.Replicas != nil &&
					*deploy.Spec.Replicas == int32(4) &&
					deploy.GetGeneration() > generation
			}, timeout, interval).Should(BeTrue())
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
				}, {
					BackendObjectReference: stnrgwv1.BackendObjectReference{
						Name: "testservice-ok",
					},
				}}
			})

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				if len(c.Clusters) == 1 && contains(c.Clusters[0].Endpoints, "10.11.12.13") &&
					len(c.Listeners) == 2 && len(c.Listeners[0].Routes) == 1 && len(c.Listeners[1].Routes) == 1 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())
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
			Expect(l.Key).NotTo(Equal(""))
			Expect(l.Cert).NotTo(Equal(""))
			Expect(l.Routes[0]).Should(Equal("testnamespace/udproute-ok"))

			Expect(conf.Clusters).To(HaveLen(1))

			c := conf.Clusters[0]

			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(8))
			Expect(c.Endpoints).Should(ContainElement("10.11.12.13"))
			Expect(c.Endpoints).Should(ContainElement("10.11.12.14"))
			Expect(c.Endpoints).Should(ContainElement("10.11.12.15"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))
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
				return s.Status == metav1.ConditionTrue
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
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute), ro)
				if err != nil || len(ro.Status.Parents) != 1 {
					return false
				}

				// should be programmed
				return ro.Status.Parents[0].ParentRef.Name == gwapiv1.ObjectName("gateway-1") &&
					ro.Status.Parents[0].ParentRef.SectionName == nil
			}, timeout, interval).Should(BeTrue())

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

		It("should survive converting the route to v1a2 route", func() {
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

			ctrl.Log.Info("deleting stunnerv1 UDPRoute")
			Expect(k8sClient.Delete(ctx, testUDPRoute)).Should(Succeed())

			ctrl.Log.Info("adding v1alpha2 UDPRoute")
			Expect(k8sClient.Create(ctx, testUDPRouteV1A2)).Should(Succeed())

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(checkConfig(ch, func(c *stnrv1.StunnerConfig) bool {
				if len(c.Listeners) == 2 &&
					(len(c.Listeners[0].Routes) == 1 || len(c.Listeners[1].Routes) == 1) &&
					len(c.Clusters) == 1 &&
					!contains(c.Clusters[0].Endpoints, "10.11.12.13") {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())

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
			Expect(l.Routes).To(HaveLen(0))

			Expect(conf.Clusters).To(HaveLen(1))
			c := conf.Clusters[0]
			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))

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
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				return true
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

			// stragely recreating the gateway lets api-server to find the public ip
			// for the gw so Ready status becomes true (should investigate this)
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

			roV1A2 := &gwapiv1a2.UDPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute), roV1A2)
				if err != nil || len(roV1A2.Status.Parents) != 1 {
					return false
				}
				s := meta.FindStatusCondition(roV1A2.Status.Parents[0].Conditions,
					string(gwapiv1.RouteConditionAccepted))
				return s != nil && s.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			Expect(roV1A2.Status.Parents).To(HaveLen(1))
			ps := roV1A2.Status.Parents[0]
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

		It("should survive masking the v1a2 route with a stunner.v1 route", func() {
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

			ctrl.Log.Info("loading UDPRoute")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, nil)

			// wait until v1a2 route gets invalidated
			Eventually(func() bool {
				roV1A2 := &gwapiv1a2.UDPRoute{}
				err := k8sClient.Get(ctx, store.GetNamespacedName(testUDPRouteV1A2), roV1A2)
				if err != nil {
					return false
				}

				for _, s := range roV1A2.Status.Parents {
					cond := meta.FindStatusCondition(s.Conditions,
						string(gwapiv1.RouteConditionAccepted))
					if cond != nil {
						if cond.Status == metav1.ConditionFalse &&
							cond.Reason == string(gwapiv1.RouteReasonPending) {
							return true
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			ctrl.Log.Info("trying to load STUNner config")
			Eventually(func() bool {
				// there is a good chance we won't get an update so we load the new config
				cl, err := cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", "",
					logger.NewLoggerFactory(stunnerLogLevel))
				Expect(err).Should(Succeed())

				c, err := cl.Load()
				Expect(err).Should(Succeed())

				if len(c.Listeners) == 2 && (len(c.Listeners[0].Routes) == 1 || len(c.Listeners[1].Routes) == 0) &&
					len(c.Clusters) == 1 {
					conf = c
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

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
			Expect(l.Routes).To(HaveLen(0))

			Expect(conf.Clusters).To(HaveLen(1))
			c := conf.Clusters[0]
			Expect(c.Name).Should(Equal("testnamespace/udproute-ok"))
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))

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
				return s.Status == metav1.ConditionTrue

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

			roV1A2 := &gwapiv1a2.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				roV1A2)).Should(Succeed())

			Expect(roV1A2.Status.Parents).To(HaveLen(1))
			ps = roV1A2.Status.Parents[0]
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
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.RouteReasonPending)))

			s = meta.FindStatusCondition(ps.Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.RouteConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			ctrl.Log.Info("deleting v1a2 UDPRoute")
			Expect(k8sClient.Delete(ctx, testUDPRouteV1A2)).Should(Succeed())
		})

		It("should stabilize", func() {
			op.Stabilize()
		})
	})

	// MULTI-GATEWAY
	Context("When creating 2 Gateways", Ordered, Label("managed"), func() {
		conf := &stnrv1.StunnerConfig{}
		var clientCtx context.Context
		var clientCancel context.CancelFunc
		var ch1, ch2 chan *stnrv1.StunnerConfig
		var cdsClient1, cdsClient2 cdsclient.Client

		BeforeAll(func() {
			// switch EDS on
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch1 = make(chan *stnrv1.StunnerConfig, 128)
			ch2 = make(chan *stnrv1.StunnerConfig, 128)
			var err error
			logger := logger.NewLoggerFactory(stunnerLogLevel)
			cdsClient1, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", "", logger)
			Expect(err).Should(Succeed())
			Expect(cdsClient1.Watch(clientCtx, ch1, false)).Should(Succeed())
			cdsClient2, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-2", "", logger)
			Expect(err).Should(Succeed())
			Expect(cdsClient2.Watch(clientCtx, ch2, false)).Should(Succeed())
		})

		AfterAll(func() {
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			clientCancel()
			close(ch1)
			close(ch2)
		})

		It("should survive loading all resources", func() {
			ctrl.Log.Info("loading Gateway 2")
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, gw2, func() error {
				testGw.Spec.DeepCopyInto(&gw2.Spec)
				gw2.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-2-listener-udp"),
					Port:     gwapiv1.PortNumber(10),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("invalid"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("dummy"),
				}}
				return nil
			})
			Expect(err).Should(Succeed())

			// UDPRoute: both gateways are parents
			ctrl.Log.Info("updating UDPRoute")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, func(current *stnrgwv1.UDPRoute) {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
				current.Spec.CommonRouteSpec = gwapiv1.CommonRouteSpec{
					ParentRefs: []gwapiv1.ParentReference{{
						Name:        "gateway-1",
						SectionName: &testutils.TestSectionName,
					}, {
						Name: "gateway-2",
					}},
				}
			})
		})

		It("should render a STUNner config for Gateway 1", func() {
			ctrl.Log.Info("trying to Get STUNner configmap", "resource", "testnamespace/gateway-1")
			Eventually(checkConfig(ch1, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) == 2 && len(c.Listeners[1].Routes) == 0 && len(c.Clusters) == 1 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())

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
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))
		})

		It("should render a STUNner config for Gateway 2", func() {
			ctrl.Log.Info("trying to Get STUNner configmap", "resource", "testnamespace/gateway-2")
			Eventually(checkConfig(ch2, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) == 1 && len(c.Listeners[0].Routes) == 1 && len(c.Clusters) == 1 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(1))

			l := conf.Listeners[0]
			Expect(l.Name).Should(Equal("testnamespace/gateway-2/gateway-2-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(10))
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

		It("should set the status of Gateway 1", func() {
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testGw), gw)
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

		It("should set the status of Gateway 2", func() {
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw2), gw2)
				if err != nil {
					return false
				}

				// should NOT be programmed (invalid listener)
				s := meta.FindStatusCondition(gw2.Status.Conditions,
					string(gwapiv1.GatewayConditionProgrammed))
				if s == nil || s.Status != metav1.ConditionFalse {
					return false
				}
				listenerStatuses := gw2.Status.Listeners
				if len(listenerStatuses) != 2 ||
					listenerStatuses[0].Name != "gateway-2-listener-udp" ||
					listenerStatuses[1].Name != "invalid" {
					return false
				}

				s = meta.FindStatusCondition(listenerStatuses[0].Conditions,
					string(gwapiv1.GatewayConditionAccepted))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(gw2.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))

			// listeners: no public gateway address so Ready status is False
			Expect(gw2.Status.Listeners).To(HaveLen(2))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionConflicted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonNoConflicts)))

			// listeners[1]: detached
			s = meta.FindStatusCondition(gw2.Status.Listeners[1].Conditions,
				string(gwapiv1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonUnsupportedProtocol)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[1].Conditions,
				string(gwapiv1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[1].Conditions,
				string(gwapiv1.ListenerConditionConflicted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonNoConflicts)))
		})

		It("should set the Route status", func() {
			ro := &stnrgwv1.UDPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute), ro)
				if err != nil || ro == nil {
					return false
				}
				return len(ro.Status.Parents) == 2
			}, timeout, interval).Should(BeTrue())

			Expect(ro.Status.Parents).To(HaveLen(2))

			ps := ro.Status.Parents[0]
			if ps.ParentRef.Name != gwapiv1.ObjectName("gateway-1") {
				ps = ro.Status.Parents[1]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
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

			ps = ro.Status.Parents[1]
			if ps.ParentRef.Name != gwapiv1.ObjectName("gateway-2") {
				ps = ro.Status.Parents[0]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-2")))
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

		It("should create a Deployment for Gateway 1", func() {
			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}
			gw := &gwapiv1.Gateway{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, deploy); err != nil || deploy == nil {
					return false
				}
				if err := k8sClient.Get(ctx, lookupKey, gw); err != nil || gw == nil {
					return false
				}
				return store.IsOwner(gw, deploy, "Gateway")
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

			Expect(store.IsOwner(gw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Merge(
					labels.Set{opdefault.AppLabelKey: opdefault.OwnedByLabelValue},
					labels.Set{opdefault.RelatedGatewayKey: testGw.GetName()},
				),
				labels.Set{opdefault.RelatedGatewayNamespace: testGw.GetNamespace()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(3))
			v, ok = labs[opdefault.AppLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.AppLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetName()))
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetNamespace()))

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
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}

			ctrl.Log.Info("trying to Get Deployment", "resource", store.GetNamespacedName(deploy))
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, store.GetNamespacedName(gw2), deploy); err != nil || deploy == nil {
					return false
				}
				if err := k8sClient.Get(ctx, store.GetNamespacedName(gw2), gw2); err != nil || gw2 == nil {
					return false
				}
				return store.IsOwner(gw2, deploy, "Gateway")
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
				labels.Merge(
					labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
					labels.Set{opdefault.RelatedGatewayKey: gw2.GetName()},
				),
				labels.Set{opdefault.RelatedGatewayNamespace: gw2.GetNamespace()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(3))
			v, ok = labs[opdefault.AppLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.AppLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetName()))
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetNamespace()))

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

		It("should remove the dataplane if disable-managed-dataplane annotation is added to Gateway 2", func() {
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Expect(k8sClient.Get(ctx, store.GetNamespacedName(gw2), gw2)).Should(Succeed())

			// apply annotation
			createOrUpdateGateway(ctx, k8sClient, gw2, func(current *gwapiv1.Gateway) {
				ann := current.GetAnnotations()
				if len(ann) == 0 {
					ann = make(map[string]string)
				}
				ann[opdefault.ManagedDataplaneDisabledAnnotationKey] = opdefault.ManagedDataplaneDisabledAnnotationValue
				current.SetAnnotations(ann)
			})

			// check
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gw2), gw2)
				if err != nil {
					return false
				}

				// should have the annotation
				_, ok := gw2.GetAnnotations()[opdefault.ManagedDataplaneDisabledAnnotationKey]
				return ok
			}, timeout, interval).Should(BeTrue())

			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      gw2.GetName(),
				Namespace: string(testutils.TestNsName),
			}}
			ctrl.Log.Info("trying to Get Deployment for Gateway 2",
				"resource", store.GetNamespacedName(deploy))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(deploy), deploy)
				return err != nil && apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should restore the dataplane if disable-managed-dataplane annotation is removed from Gateway 2", func() {
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Expect(k8sClient.Get(ctx, store.GetNamespacedName(gw2), gw2)).Should(Succeed())

			// make sure
			v, ok := gw2.GetAnnotations()[opdefault.ManagedDataplaneDisabledAnnotationKey]
			Expect(ok).Should(BeTrue())
			Expect(v).NotTo(Equal(opdefault.ManagedDataplaneDisabledAnnotationKey))

			// remove annotation
			createOrUpdateGateway(ctx, k8sClient, gw2, func(current *gwapiv1.Gateway) {
				ann := current.GetAnnotations()
				if len(ann) == 0 {
					ann = make(map[string]string)
				}
				delete(ann, opdefault.ManagedDataplaneDisabledAnnotationKey)
				current.SetAnnotations(ann)
			})

			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      gw2.GetName(),
				Namespace: string(testutils.TestNsName),
			}}
			ctrl.Log.Info("trying to Get Deployment for Gateway 2",
				"resource", store.GetNamespacedName(deploy))

			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(deploy), deploy)
				return err != nil && deploy != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("should survive deleting Gateway 2", func() {
			ctrl.Log.Info("deleting Gateway 2")
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Expect(k8sClient.Delete(ctx, gw2)).Should(Succeed())

			// wait until route status gets updated
			ro := &stnrgwv1.UDPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(testUDPRoute), ro)
				if err != nil || ro == nil {
					return false
				}

				ps := ro.Status.Parents[0]
				if ps.ParentRef.Name != gwapiv1.ObjectName("gateway-2") {
					ps = ro.Status.Parents[1]
				}

				// fmt.Println("++++++++++++++++++++++")
				// fmt.Printf("%#v\n", ps)
				// fmt.Printf("%#v\n", ps.Conditions)
				// fmt.Println("++++++++++++++++++++++")

				s := meta.FindStatusCondition(ps.Conditions,
					string(gwapiv1.RouteConditionAccepted))
				if s != nil && s.Status == metav1.ConditionFalse {
					return true
				}

				return false
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(2))

			ps := ro.Status.Parents[0]
			if ps.ParentRef.Name != gwapiv1.ObjectName("gateway-1") {
				ps = ro.Status.Parents[1]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
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

			ps = ro.Status.Parents[1]
			if ps.ParentRef.Name != gwapiv1.ObjectName("gateway-2") {
				ps = ro.Status.Parents[0]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-2")))
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

		It("should stabilize", func() {
			op.Stabilize()
		})
	})

	// MULTI-GATEWAYCLASS
	Context("When creating 2 GatewayClasses and Gateways", Ordered, Label("managed"), func() {
		conf := &stnrv1.StunnerConfig{}
		var clientCtx context.Context
		var clientCancel context.CancelFunc
		var ch1, ch2 chan *stnrv1.StunnerConfig
		var cdsClient1, cdsClient2 cdsclient.Client

		BeforeAll(func() {
			// switch EDS on
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch1 = make(chan *stnrv1.StunnerConfig, 128)
			ch2 = make(chan *stnrv1.StunnerConfig, 128)
			var err error
			logger := logger.NewLoggerFactory(stunnerLogLevel)
			cdsClient1, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", "", logger)
			Expect(err).Should(Succeed())
			Expect(cdsClient1.Watch(clientCtx, ch1, false)).Should(Succeed())
			cdsClient2, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-2", "", logger)
			Expect(err).Should(Succeed())
			Expect(cdsClient2.Watch(clientCtx, ch2, false)).Should(Succeed())
		})

		AfterAll(func() {
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			clientCancel()
			close(ch1)
			close(ch2)
		})

		It("should survive loading all resources", func() {
			ctrl.Log.Info("loading GatewayClass 2")
			gc2 := &gwapiv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{
				Name: "gateway-class-2",
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, gc2, func() error {
				testGwClass.Spec.DeepCopyInto(&gc2.Spec)
				gc2.Spec.ParametersRef = &gwapiv1.ParametersReference{
					Group:     gwapiv1.Group(stnrgwv1.GroupVersion.Group),
					Kind:      gwapiv1.Kind("GatewayConfig"),
					Name:      "gateway-config-2",
					Namespace: &testutils.TestNsName,
				}

				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("loading GatewayConfig 2")
			realm := "testrealm-2"
			dataplane := "dataplane-2"
			gwConf2 := &stnrgwv1.GatewayConfig{ObjectMeta: metav1.ObjectMeta{
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

			ctrl.Log.Info("loading Dataplane 2")
			dp2 := &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
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
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			_, err = ctrlutil.CreateOrUpdate(ctx, k8sClient, gw2, func() error {
				testGw.Spec.DeepCopyInto(&gw2.Spec)
				gw2.Spec.GatewayClassName = gwapiv1.ObjectName("gateway-class-2")
				gw2.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-2-listener-udp"),
					Port:     gwapiv1.PortNumber(10),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				return nil
			})
			Expect(err).Should(Succeed())

			// UDPRoute: both gateways are parents
			ctrl.Log.Info("updating UDPRoute")
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, func(current *stnrgwv1.UDPRoute) {
				testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
				current.Spec.CommonRouteSpec = gwapiv1.CommonRouteSpec{
					ParentRefs: []gwapiv1.ParentReference{{
						Name:        "gateway-1",
						SectionName: &testutils.TestSectionName,
					}, {
						Name: "gateway-2",
					}},
				}
			})
		})

		It("should render a STUNner config for Gateway 1", func() {
			ctrl.Log.Info("trying to Get STUNner configmap", "resource", "testnamespace/gateway-1")
			Eventually(checkConfig(ch1, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) == 2 && len(c.Listeners[1].Routes) == 0 && len(c.Clusters) == 1 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())

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
			Expect(c.Type).Should(Equal("STATIC"))
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))
		})

		It("should render a STUNner config for Gateway 2", func() {
			ctrl.Log.Info("trying to Get STUNner configmap", "resource", "testnamespace/gateway-2")
			Eventually(checkConfig(ch2, func(c *stnrv1.StunnerConfig) bool {
				// conf should have valid listener confs
				if len(c.Listeners) == 1 && len(c.Clusters) == 1 {
					conf = c
					return true
				}
				return false
			}), timeout, interval).Should(BeTrue())

			Expect(conf.Listeners).To(HaveLen(1))

			l := conf.Listeners[0]
			Expect(l.Name).Should(Equal("testnamespace/gateway-2/gateway-2-listener-udp"))
			Expect(l.Protocol).Should(Equal("TURN-UDP"))
			Expect(l.Port).Should(Equal(10))
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
		})

		It("should set the status of GatewayClass 2", func() {
			gc := &gwapiv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{
				Name: "gateway-class-2",
			}}
			// wait until status gets updated
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gc), gc)
				if err != nil || gc == nil {
					return false
				}

				s := meta.FindStatusCondition(gc.Status.Conditions,
					string(gwapiv1.GatewayClassConditionStatusAccepted))
				if s != nil && s.Status == metav1.ConditionTrue {
					return true
				}

				return false
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

		It("should set the status of Gateway 1", func() {
			gw := &gwapiv1.Gateway{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testGw), gw)
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

		It("should set the status of Gateway 2", func() {
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
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
					string(gwapiv1.GatewayConditionProgrammed))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				// should get a public IP
				listenerStatuses := gw2.Status.Listeners
				if len(listenerStatuses) != 1 || listenerStatuses[0].Name != "gateway-2-listener-udp" {
					return false
				}

				s = meta.FindStatusCondition(listenerStatuses[0].Conditions,
					string(gwapiv1.GatewayConditionAccepted))
				if s == nil || s.Status != metav1.ConditionTrue {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(gw2.Status.Conditions).To(HaveLen(2))

			s := meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1.GatewayConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayConditionAccepted)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			s = meta.FindStatusCondition(gw2.Status.Conditions,
				string(gwapiv1.GatewayConditionProgrammed))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.GatewayConditionProgrammed)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))

			Expect(gw2.Status.Listeners).To(HaveLen(1))

			// listener[0]: OK
			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionAccepted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonAccepted)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionResolvedRefs))
			Expect(s).NotTo(BeNil())
			Expect(s.Type).Should(
				Equal(string(gwapiv1.ListenerConditionResolvedRefs)))
			Expect(s.Status).Should(Equal(metav1.ConditionTrue))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonResolvedRefs)))

			s = meta.FindStatusCondition(gw2.Status.Listeners[0].Conditions,
				string(gwapiv1.ListenerConditionConflicted))
			Expect(s).NotTo(BeNil())
			Expect(s.Status).Should(Equal(metav1.ConditionFalse))
			Expect(s.Reason).Should(Equal(string(gwapiv1.ListenerReasonNoConflicts)))
		})

		It("should set the Route status", func() {
			ro := &stnrgwv1.UDPRoute{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute), ro)
				if err != nil || ro == nil {
					return false
				}
				return len(ro.Status.Parents) == 2
			}, timeout, interval).Should(BeTrue())

			Expect(ro.Status.Parents).To(HaveLen(2))

			ps := ro.Status.Parents[0]
			if ps.ParentRef.Name != gwapiv1.ObjectName("gateway-1") {
				ps = ro.Status.Parents[1]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
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

			ps = ro.Status.Parents[1]
			if ps.ParentRef.Name != gwapiv1.ObjectName("gateway-2") {
				ps = ro.Status.Parents[0]
			}

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-2")))
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

		It("should create a Deployment for Gateway 1", func() {
			lookupKey := store.GetNamespacedName(testGw)
			deploy := &appv1.Deployment{}
			gw := &gwapiv1.Gateway{}

			ctrl.Log.Info("trying to Get Deployment", "resource", lookupKey)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, deploy); err != nil || deploy == nil {
					return false
				}
				if err := k8sClient.Get(ctx, lookupKey, gw); err != nil || gw == nil {
					return false
				}
				return store.IsOwner(gw, deploy, "Gateway")
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

			Expect(store.IsOwner(gw, deploy, "Gateway")).Should(BeTrue(), "ownerRef")

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Merge(
					labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
					labels.Set{opdefault.RelatedGatewayKey: testGw.GetName()},
				),
				labels.Set{opdefault.RelatedGatewayNamespace: testGw.GetNamespace()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(3))
			v, ok = labs[opdefault.AppLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.AppLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetName()))
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(testGw.GetNamespace()))

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
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
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
				if err != nil || deploy == nil {
					return false
				}

				// Get gw2 so that the owner-ref UID is filled in
				err = k8sClient.Get(ctx, store.GetNamespacedName(gw2), gw2)
				return err == nil && store.IsOwner(gw2, deploy, "Gateway")
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

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Merge(
					labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
					labels.Set{opdefault.RelatedGatewayKey: gw2.GetName()},
				),
				labels.Set{opdefault.RelatedGatewayNamespace: gw2.GetNamespace()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(3))
			v, ok = labs[opdefault.AppLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.AppLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetName()))
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetNamespace()))

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

		It("should stabilize", func() {
			op.Stabilize()
		})

		It("should retain Deployment labels/annotations and propagate Gateway label/annotation updates to the Deployment", func() {
			// existing labels/annotations should not be overwritten unless in conflict
			ctrl.Log.Info("updating metadata on Deployment 2")
			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}

			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, deploy, func() error {
				labels := deploy.GetLabels()
				labels["valid-deploy-label"] = "valid-deploy-label-value" // will be ertained on the deployment
				deploy.SetLabels(labels)

				as := deploy.GetAnnotations()
				as["valid-deploy-ann"] = "valid-deploy-ann-value" // will be retained on the deployment/pod
				deploy.SetAnnotations(as)

				as = deploy.Spec.Template.GetAnnotations()
				as["valid-deploy-pod-ann"] = "valid-deploy-pod-ann-value" // will be retained on the deployment/pod
				deploy.Spec.Template.SetAnnotations(as)

				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("waiting for Deployment update to complete",
				"resource", store.GetNamespacedName(deploy))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(deploy), deploy)
				if err != nil || deploy == nil {
					return false
				}
				labels := deploy.GetLabels()
				_, ok := labels["valid-deploy-label"]
				return ok
			}, timeout, interval).Should(BeTrue())
			Expect(deploy).NotTo(BeNil(), "Deployment updated")

			ctrl.Log.Info("updating metadata on Gateway 2")
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}

			_, err = ctrlutil.CreateOrUpdate(ctx, k8sClient, gw2, func() error {
				gw2.SetLabels(map[string]string{
					"stunner.l7mp.io/owned-by": "dummy-owner",          // will be overwritten on the deployment
					"valid-gw-label":           "valid-gw-label-value", // will appear on the deployment/pod
				})
				gw2.SetAnnotations(map[string]string{
					"stunner.l7mp.io/related-gateway-name": "dummy-ns/dummy-name", // will be overwritten
					"valid-gw-ann":                         "valid-gw-ann-value",  // will not appear
				})

				return nil
			})
			Expect(err).Should(Succeed())

			ctrl.Log.Info("waiting for Gateway update to complete",
				"resource", store.GetNamespacedName(gw2))
			// This also gets the owner-ref UID filled in, which we need for owbership
			// testing on the Deployment
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(gw2), gw2)
				if err != nil || gw2 == nil {
					return false
				}

				labels := gw2.GetLabels()
				_, ok := labels["valid-gw-label"]
				return ok
			}, timeout, interval).Should(BeTrue())
			Expect(gw2).NotTo(BeNil(), "Gateway2 updated")

			ctrl.Log.Info("waiting for Deployment update to complete",
				"resource", store.GetNamespacedName(deploy))
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(gw2), deploy)
				if err != nil || deploy == nil || !store.IsOwner(gw2, deploy, "Gateway") {
					return false
				}
				labels := deploy.GetLabels()
				v, ok := labels["valid-gw-label"]
				return ok && v == "valid-gw-label-value"
			}, timeout, interval).Should(BeTrue())
			Expect(deploy).NotTo(BeNil(), "Deployment rendered")

			// metadata
			// labels: 3 mandatory + 1 label from the gw + 1 label from the deployment
			labs := deploy.GetLabels()
			Expect(labs).To(HaveLen(5))

			// mandatory labels
			v, ok := labs[opdefault.OwnedByLabelKey]
			Expect(ok).Should(BeTrue(), "app label")
			Expect(v).Should(Equal(opdefault.OwnedByLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "labels: related")
			Expect(v).Should(Equal(gw2.GetName()), "related-gw label value")
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue(), "labels: related namespace")
			Expect(v).Should(Equal(gw2.GetNamespace()), "related-gw-namespace label value")

			// label from the gw
			v, ok = labs["valid-gw-label"] // testGw has it, hw2 doesn't
			Expect(ok).Should(BeTrue(), "gw label")
			Expect(v).Should(Equal("valid-gw-label-value"), "gw label value")

			// label from the deployment
			v, ok = labs["valid-deploy-label"] // testGw has it, hw2 doesn't
			Expect(ok).Should(BeTrue(), "deploy label")
			Expect(v).Should(Equal("valid-deploy-label-value"), "deploy label value")

			// annotations: 1 mandatory + 1 ann from the gw + 1 ann from the deployment
			as := deploy.GetAnnotations()
			Expect(as).To(HaveLen(3))

			// mandatory
			v, ok = as[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "related-gw annotation")
			Expect(v).Should(Equal(store.GetObjectKey(gw2)), "related-gw annotation value")

			// annotation from the gw
			v, ok = as["valid-gw-ann"]
			Expect(ok).Should(BeTrue(), "gw ann")
			Expect(v).Should(Equal("valid-gw-ann-value"), "gw ann value")

			// annotation from the deployment
			v, ok = as["valid-deploy-ann"]
			Expect(ok).Should(BeTrue(), "deploy ann")
			Expect(v).Should(Equal("valid-deploy-ann-value"), "deploy ann value")

			// check the label selector
			labelSelector := deploy.Spec.Selector
			Expect(labelSelector).NotTo(BeNil(), "selector set")

			selector, err := metav1.LabelSelectorAsSelector(labelSelector)
			Expect(err).To(Succeed())

			// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
			// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			labelToMatch := labels.Merge(
				labels.Merge(
					labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
					labels.Set{opdefault.RelatedGatewayKey: gw2.GetName()},
				),
				labels.Set{opdefault.RelatedGatewayNamespace: gw2.GetNamespace()},
			)
			Expect(selector.Matches(labelToMatch)).Should(BeTrue(), "selector matches")

			podTemplate := &deploy.Spec.Template
			labs = podTemplate.GetLabels()
			Expect(labs).To(HaveLen(3))
			v, ok = labs[opdefault.AppLabelKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(opdefault.AppLabelValue))
			v, ok = labs[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetName()))
			v, ok = labs[opdefault.RelatedGatewayNamespace]
			Expect(ok).Should(BeTrue())
			Expect(v).Should(Equal(gw2.GetNamespace()))

			// deployment selector matches pod template
			Expect(selector.Matches(labels.Set(labs))).Should(BeTrue())

			// pod-level annotations:
			as = deploy.Spec.Template.GetAnnotations()
			Expect(as).To(HaveLen(2))

			// mandatory
			v, ok = as[opdefault.RelatedGatewayKey]
			Expect(ok).Should(BeTrue(), "related-gw annotation")
			Expect(v).Should(Equal(store.GetObjectKey(gw2)), "related-gw annotation value")

			// annotation from the gw
			_, ok = as["valid-gw-ann"]
			Expect(ok).Should(BeFalse(), "gw ann")

			// annotation from the deployment
			v, ok = as["valid-deploy-pod-ann"]
			Expect(ok).Should(BeTrue(), "deploy pod ann")
			Expect(v).Should(Equal("valid-deploy-pod-ann-value"), "deploy pod ann value")
		})

		It("should update only Gateway 2 when Dataplane 2 changes", func() {
			ctrl.Log.Info("updating  Dataplane 2")
			dp2 := &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: "dataplane-2",
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, dp2, func() error {
				testDataplane.Spec.DeepCopyInto(&dp2.Spec)
				dp2.Spec.Image = "testimage-3"
				dp2.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{
					Name: "testpullsecret1",
				}, {
					Name: "testpullsecret2",
				}}
				runAsNonRoot := true
				dp2.Spec.SecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: &runAsNonRoot}
				dp2.Spec.ContainerSecurityContext = &corev1.SecurityContext{RunAsNonRoot: &runAsNonRoot}
				dp2.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
					TopologyKey:       "a",
					MaxSkew:           int32(12),
					WhenUnsatisfiable: corev1.DoNotSchedule,
				}, {
					TopologyKey:       "b",
					MaxSkew:           int32(21),
					WhenUnsatisfiable: corev1.ScheduleAnyway,
				}}
				grace := int64(10)
				dp2.Spec.TerminationGracePeriodSeconds = &grace
				selector := corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{{
						MatchExpressions: []corev1.NodeSelectorRequirement{{
							Key:      "dummy_key",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"dummy_val"},
						}},
					}},
				}
				nodeAff := corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &selector,
				}
				aff := corev1.Affinity{NodeAffinity: &nodeAff}
				dp2.Spec.Affinity = &aff
				return nil
			})
			Expect(err).Should(Succeed())

			// wait till Dataplane is uploaded
			dp2 = &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: "dataplane-2",
			}}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, store.GetNamespacedName(dp2), dp2)
				if err != nil || dp2 == nil {
					return false
				}

				dpSpec := &dp2.Spec
				return dpSpec.Image == "testimage-3"
			}, timeout, interval).Should(BeTrue())

			deploy := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
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
				if len(podSpec.Containers) != 1 || podSpec.Containers[0].Image != "testimage-3" {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// check the other change: ImagePullSecrets
			podSpec := &deploy.Spec.Template.Spec
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].Image).Should(Equal("testimage-3"))
			Expect(podSpec.Containers[0].SecurityContext).NotTo(BeNil())
			Expect(podSpec.Containers[0].SecurityContext.RunAsNonRoot).NotTo(BeNil())
			Expect(*podSpec.Containers[0].SecurityContext.RunAsNonRoot).To(BeTrue())

			Expect(podSpec.ImagePullSecrets).To(HaveLen(2))
			Expect(podSpec.ImagePullSecrets[0].Name).Should(Equal("testpullsecret1"))
			Expect(podSpec.ImagePullSecrets[1].Name).Should(Equal("testpullsecret2"))
			Expect(podSpec.TopologySpreadConstraints).To(HaveLen(2))
			Expect(podSpec.TopologySpreadConstraints[0].TopologyKey).Should(Equal("a"))
			Expect(podSpec.TopologySpreadConstraints[0].MaxSkew).Should(Equal(int32(12)))
			Expect(podSpec.TopologySpreadConstraints[0].WhenUnsatisfiable).Should(Equal(corev1.DoNotSchedule))
			Expect(podSpec.TopologySpreadConstraints[1].TopologyKey).Should(Equal("b"))
			Expect(podSpec.TopologySpreadConstraints[1].MaxSkew).Should(Equal(int32(21)))
			Expect(podSpec.TopologySpreadConstraints[1].WhenUnsatisfiable).Should(Equal(corev1.ScheduleAnyway))
			Expect(podSpec.SecurityContext).NotTo(BeNil())
			Expect(podSpec.SecurityContext.RunAsNonRoot).NotTo(BeNil())
			Expect(*podSpec.SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(podSpec.Affinity).NotTo(BeNil())
			Expect(podSpec.Affinity.NodeAffinity).NotTo(BeNil())
			req := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
			Expect(req).NotTo(BeNil())
			Expect(req.NodeSelectorTerms).To(HaveLen(1))
			Expect(req.NodeSelectorTerms[0].MatchExpressions).To(HaveLen(1))
			Expect(req.NodeSelectorTerms[0].MatchExpressions[0].Key).To(Equal("dummy_key"))
			Expect(req.NodeSelectorTerms[0].MatchExpressions[0].Operator).To(Equal(corev1.NodeSelectorOpIn))
			Expect(req.NodeSelectorTerms[0].MatchExpressions[0].Values).To(HaveLen(1))
			Expect(req.NodeSelectorTerms[0].MatchExpressions[0].Values[0]).To(Equal("dummy_val"))

			// gateway 1 should not change
			deploy = &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
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

			podSpec = &deploy.Spec.Template.Spec
			Expect(podSpec.ImagePullSecrets).To(HaveLen(0))
			Expect(podSpec.TopologySpreadConstraints).To(HaveLen(0))
		})

		It("should survive a full cleanup for the Gateway-1 hierarchy", func() {
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

			// ctrl.Log.Info("deleting Endpoints")
			// Expect(k8sClient.Delete(ctx, testEndpoint)).Should(Succeed())

			ctrl.Log.Info("deleting Dataplane")
			Expect(k8sClient.Delete(ctx, testDataplane)).Should(Succeed())
		})

		It("should survive a full cleanup for the Gateway-2 hierarchy", func() {
			ctrl.Log.Info("deleting GatewayClass")
			gc2 := &gwapiv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{
				Name: "gateway-class-2",
			}}
			Expect(k8sClient.Delete(ctx, gc2)).Should(Succeed())

			ctrl.Log.Info("deleting GatewayConfig")
			gwConf2 := &stnrgwv1.GatewayConfig{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-config-2",
				Namespace: string(testutils.TestNsName),
			}}
			Expect(k8sClient.Delete(ctx, gwConf2)).Should(Succeed())

			ctrl.Log.Info("deleting Gateway")
			gw2 := &gwapiv1.Gateway{ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-2",
				Namespace: string(testutils.TestNsName),
			}}
			Expect(k8sClient.Delete(ctx, gw2)).Should(Succeed())

			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
		})
	})
}
