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
	// "time"
	// "reflect"
	// "testing"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	// v1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/client"

	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// make sure we use fmt
var _ = fmt.Sprintf("whatever: %d", 1)

// GatewayClass + GatewayConfig + Gateway should be enough to render a valid STUNner conf
var _ = Describe("Minimal-test", func() {
	ctx, cancel := context.WithCancel(context.Background())
	// ctx := context.TODO()
	conf := &stunnerconfv1alpha1.StunnerConfig{}

	Context("When creating a minimal set of API resources", func() {

		It("should bootstrap the operator runtime successfully", func() {
			setup(ctx, k8sClient)
		})

		It("should load the minimal config", func() {
			ctrl.Log.Info("loading GatewayClass")
			Expect(k8sClient.Create(ctx, &testutils.TestGwClass)).Should(Succeed())
			ctrl.Log.Info("loading GatewayConfig")
			Expect(k8sClient.Create(ctx, &testutils.TestGwConfig)).Should(Succeed())
			ctrl.Log.Info("loading Gateway")
			Expect(k8sClient.Create(ctx, &testutils.TestGw)).Should(Succeed())
		})

		It("Should successfully load a gateway-config", func() {

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

		It("Should successfully render a STUNner ConfigMap", func() {

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

		It("Should yield a ConfigMap that can be successfully unpacked", func() {

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

		It("Should yield a STUNner config with exactly 2 listeners", func() {

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

		})

		It("Should yield a STUNner config with correct params", func() {

			Expect(conf).NotTo(BeNil(), "STUNner config rendered")
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

		It("Should let the controller shut down", func() {
			cancel()
		})

	})
})
