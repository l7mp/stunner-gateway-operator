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

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func testLegacyModeEndpointController() {
	// WITH EDS, WITHOUT RELAY-CLUSTER-IP
	Context("When creating a minimal set of API resources (EDS ENABLED, RELAY-TO-CLUSTER-IP ENABLED, ENDPOINT-CONTROLLER-ENABLED)", Ordered, Label("legacy"), func() {
		conf := &stnrconfv1.StunnerConfig{}

		It("should survive loading a minimal config", func() {
			// switch EDS off
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true

			createOrUpdateGatewayClass(testGwClass, nil)
			createOrUpdateGatewayConfig(testGwConfig, nil)
			createOrUpdateGateway(testGw, nil)
			createOrUpdateUDPRoute(testUDPRoute, nil)
			createOrUpdateService(testSvc, nil)
			createOrUpdateEndpoints(testEndpoint, nil)

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
					len(c.Clusters) == 1 && len(c.Clusters[0].Endpoints) == 5 {
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
			Expect(c.Endpoints).To(HaveLen(5))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.4"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.5"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.6"))
			Expect(c.Endpoints).Should(ContainElement("1.2.3.7"))

			svc := store.Services.GetObject(types.NamespacedName{
				Namespace: "testnamespace", Name: "testservice-ok"})
			Expect(c.Endpoints).Should(ContainElement(svc.Spec.ClusterIP))

		})

		It("should set the Route status", func() {
			ro := &stnrgwv1.UDPRoute{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&testutils.TestUDPRoute),
				ro)).Should(Succeed())

			Expect(ro.Status.Parents).To(HaveLen(1))
			ps := ro.Status.Parents[0]

			Expect(ps.ParentRef.Group).To(HaveValue(Equal(gwapiv1.Group("gateway.networking.k8s.io"))))
			Expect(ps.ParentRef.Kind).To(HaveValue(Equal(gwapiv1.Kind("Gateway"))))
			Expect(ps.ParentRef.Namespace).To(BeNil())
			Expect(ps.ParentRef.Name).To(Equal(gwapiv1.ObjectName("gateway-1")))
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

		// service delete deletes its enpoint
		// ctrl.Log.Info("deleting Endpoint")
		// Expect(k8sClient.Delete(ctx, testEndpoint)).Should(Succeed())
	})
}
