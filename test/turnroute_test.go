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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
	cdsclient "github.com/l7mp/stunner/v2/pkg/config/client"
	"github.com/l7mp/stunner/v2/pkg/logger"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

// testTURNRouteManaged exercises TURNRoute handling in managed mode via the CDS server: the tunnel
// cluster, the tunnel auth policy from the Secret, the route status, and a Secret update.
func testTURNRouteManaged() {
	Context("When creating TURNRoutes (MANAGED)", Ordered, Label("managed"), func() {
		var clientCtx context.Context
		var clientCancel context.CancelFunc
		var ch chan *stnrapiv2.StunnerConfig
		var cdsClient cdsclient.Client

		BeforeAll(func() {
			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch = make(chan *stnrapiv2.StunnerConfig, 128)
			var err error
			log := logger.NewLoggerFactory(stunnerLogLevel)
			cdsClient, err = cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", nil, log)
			Expect(err).Should(Succeed())
			Expect(cdsClient.Watch(clientCtx, ch, false)).Should(Succeed())
		})

		AfterAll(func() {
			clientCancel()
			// do not close(ch): the watcher may deliver an in-flight config after the
			// cancel and would panic on a closed channel
		})

		It("should survive loading the base resources and a TURNRoute", func() {
			createOrUpdateGatewayClass(ctx, k8sClient, testGwClass, nil)
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
			createOrUpdateSecret(ctx, k8sClient, testTunnelSecret, nil)
			createOrUpdateTURNRoute(ctx, k8sClient, testTURNRoute, nil)

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

		It("should render the tunnel cluster and its auth policy", func() {
			var conf *stnrapiv2.StunnerConfig
			Eventually(checkConfig(ch, func(c *stnrapiv2.StunnerConfig) bool {
				// the cluster, attached to the listener of the route
				if findCluster(c, "testnamespace/turnroute-ok/turn") == nil ||
					!listenerHasCluster(c, "testnamespace/gateway-1/gateway-1-listener-udp",
						"testnamespace/turnroute-ok/turn") {
					return false
				}
				conf = c
				return true
			}), timeout, interval).Should(BeTrue())

			c := findCluster(conf, "testnamespace/turnroute-ok/turn")
			Expect(c.Tunnel).NotTo(BeNil())
			Expect(c.Tunnel.URL).Should(Equal(testTURNRoute.Spec.Tunnel.URI))
			Expect(c.Tunnel.Auth).Should(Equal(c.Name))

			auth, err := conf.GetAuthConfig(c.Name)
			Expect(err).Should(Succeed())
			Expect(auth.Credentials["username"]).Should(Equal("tunnel-user"))
		})

		It("should set the TURNRoute status", func() {
			ro := &stnrgwv1.TURNRoute{}
			lookupKey := store.GetNamespacedName(testTURNRoute)
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, lookupKey, ro); err != nil {
					return false
				}
				d := routeAcceptedCondition(ro.Status.Parents)
				return d != nil && d.Status == metav1.ConditionTrue
			}, timeout, interval).Should(BeTrue())

			d := meta.FindStatusCondition(ro.Status.Parents[0].Conditions,
				string(gwapiv1.RouteConditionResolvedRefs))
			Expect(d).NotTo(BeNil(), "resolved-refs condition")
			Expect(d.Status).Should(Equal(metav1.ConditionTrue), "resolved-refs")
		})

		It("should follow an update of the Secret", func() {
			createOrUpdateSecret(ctx, k8sClient, testTunnelSecret, func(current *corev1.Secret) {
				current.Data["username"] = []byte("new-tunnel-user")
			})

			Eventually(checkConfig(ch, func(c *stnrapiv2.StunnerConfig) bool {
				auth, err := c.GetAuthConfig("testnamespace/turnroute-ok/turn")
				return err == nil && auth.Credentials["username"] == "new-tunnel-user"
			}), timeout, interval).Should(BeTrue())
		})

		It("should survive deleting the TURNRoute resources", func() {
			ctrl.Log.Info("deleting the TURNRoute")
			Expect(k8sClient.Delete(ctx, testTURNRoute)).Should(Succeed())

			ctrl.Log.Info("deleting the tunnel Secret")
			Expect(k8sClient.Delete(ctx, testTunnelSecret)).Should(Succeed())

			ctrl.Log.Info("deleting the Gateway")
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())
		})
	})
}
