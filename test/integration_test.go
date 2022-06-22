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
	// "reflect"
	// "testing"
	// "fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	// v1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/client"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var _ = Describe("E2E test", func() {

	Context("When creating API resources", func() {

		It("Should successfully render a STUNner ConfigMap", func() {
			By("By creating a minimal config")
			// GatewayClass + GatewayConfig + Gateway is enough to render a STUNner conf

			// fmt.Printf("%#v\n", k8sClient)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			setup(ctx, k8sClient)

			ctrl.Log.Info("loading GatewayClass")
			Expect(k8sClient.Create(ctx, &testutils.TestGwClass)).Should(Succeed())

			ctrl.Log.Info("reading back GatewayClass")
			lookupKey := types.NamespacedName{
				Name:      testutils.TestGwClass.GetName(),
				Namespace: testutils.TestGwClass.GetNamespace(),
			}
			gwClass := &gatewayv1alpha2.GatewayClass{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, gwClass)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(gwClass.GetName()).To(Equal(testutils.TestGwClass.GetName()),
				"GatewayClass name")
			Expect(gwClass.GetNamespace()).To(Equal(testutils.TestGwClass.GetNamespace()),
				"GatewayClass namespace")

			ctrl.Log.Info("loading GatewayConfig")
			Expect(k8sClient.Create(ctx, &testutils.TestGwConfig)).Should(Succeed())
			ctrl.Log.Info("loading Gateway")
			Expect(k8sClient.Create(ctx, &testutils.TestGw)).Should(Succeed())

			lookupKey = types.NamespacedName{
				Name:      "stunner-config", // test GatewayConfig rewrites DefaultConfigMapName
				Namespace: string(testutils.TestNs),
			}
			createdConfigMap := &corev1.ConfigMap{}

			// We'll need to retry getting this newly created ConfigMap, given that
			// creation may not immediately happen.
			ctrl.Log.Info("trying to Get STUNner configmap", "resource",
				lookupKey)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, createdConfigMap)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			// Let's make sure our Schedule string value was properly converted/handled.
			Expect(createdConfigMap).NotTo(BeNil(), "STUNner config rendered")

		})
	})
})
