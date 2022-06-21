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

package controllers

import (
	"context"
	// "reflect"
	// "testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	// v1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	// "k8s.io/client-go/kubernetes/scheme"
	// runtime "k8s.io/apimachinery/pkg/runtime"
	// "sigs.k8s.io/controller-runtime/pkg/client"

	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var _ = Describe("E2E test", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)
	Context("When creating API resources", func() {
		It("Should successfully render a STUNner ConfigMap", func() {
			By("By creating a minimal config")
			// GatewayClass + GatewayConfig + Gateway is enough to render a STUNner conf
			ctx := context.Background()

			Expect(k8sClient.Create(ctx, &testutils.TestGwClass)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &testutils.TestGwConfig)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &testutils.TestGw)).Should(Succeed())

			configMapLookupKey := types.NamespacedName{
				Name:      config.DefaultConfigMapName,
				Namespace: string(testutils.TestNs),
			}
			createdConfigMap := &corev1.ConfigMap{}

			// We'll need to retry getting this newly created ConfigMap, given that
			// creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, configMapLookupKey, createdConfigMap)
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
