package integration

import (
	"context"
	"net"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
	cdsclient "github.com/l7mp/stunner/v2/pkg/config/client"
	"github.com/l7mp/stunner/v2/pkg/logger"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/app"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// resetStores empties the global stores, so that an operator instance started afterwards begins
// the way a fresh process does: with nothing but what its own controllers list from the cache.
func resetStores() {
	for _, st := range []store.Store{
		store.GatewayClasses, store.GatewayConfigs, store.Gateways, store.UDPRoutes,
		store.UDPRoutesGwAPI, store.TCPRoutes, store.TCPRoutesGwAPI, store.Services,
		store.StaticServices, store.Endpoints, store.EndpointSlices, store.ConfigMaps,
		store.Deployments, store.DaemonSets, store.Dataplanes, store.Nodes, store.Namespaces,
		store.TLSSecrets, store.AuthSecrets,
	} {
		st.Reset(nil)
	}
}

// haOperatorTest exercises the leader-only serving of the operator with the real leader
// election against the envtest API server: a standby, whose Lease is held by someone else,
// serves nothing; the leader, once the Lease is free, serves a complete config on its first
// push; and a restarted leader serves a byte-identical config, once.
func haOperatorTest() {
	Context("When running the operator with leader election", Ordered, Label("ha"), func() {
		var (
			clientCtx    context.Context
			clientCancel context.CancelFunc
			ch           chan *stnrapiv1.StunnerConfig
			lease        *coordinationv1.Lease
			lastConfig   *stnrapiv1.StunnerConfig
			cdsAddr      string
		)

		leaderElection := func(c *app.Config) {
			c.LeaderElection = true
			c.LeaderElectionNamespace = testNs.GetName()
			c.LeaseDuration = ptr.To(2 * time.Second)
			c.RenewDeadline = ptr.To(time.Second)
			c.RetryPeriod = ptr.To(200 * time.Millisecond)
			// the gate must open by every controller reporting, not by the timeout
			c.StartupRenderTimeout = 20 * time.Second
		}

		cdsPortOpen := func() bool {
			conn, err := net.DialTimeout("tcp", cdsServerAddr, 100*time.Millisecond)
			if err != nil {
				return false
			}
			conn.Close()
			return true
		}

		// receive reads the next config from the watch channel, or nil after the timeout
		receive := func(d time.Duration) *stnrapiv1.StunnerConfig {
			select {
			case c := <-ch:
				return c
			case <-time.After(d):
				return nil
			}
		}

		// complete means the route's cluster is rendered and attached to the listener the
		// route names: a render before the route controller reported would lack both
		complete := func(c *stnrapiv1.StunnerConfig) bool {
			if c == nil || len(c.Listeners) != 2 || len(c.Clusters) != 1 {
				return false
			}
			for _, l := range c.Listeners {
				if len(l.Routes) == 1 && l.Routes[0] == c.Clusters[0].Name {
					return true
				}
			}
			return false
		}

		BeforeAll(func() {
			config.EndpointSliceAvailable = true
			config.DataplaneMode = config.DataplaneModeManaged
			InitResources()
			ctx, cancel = context.WithCancel(context.Background())
		})

		AfterAll(func() {
			if clientCancel != nil {
				clientCancel()
			}
		})

		It("should be possible to create the resources before any operator runs", func() {
			// envtest runs no garbage collector: a Deployment owned by an earlier phase's
			// Gateway lingers, and it would make the Deployment checks below meaningless
			dp := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: testNs.GetName(),
				Name: testGw.GetName()}}
			if err := k8sClient.Delete(ctx, dp); err != nil {
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKeyFromObject(dp), dp))
			}, timeout, interval).Should(BeTrue())

			createOrUpdateGatewayClass(ctx, k8sClient, testGwClass, nil)
			createOrUpdateGatewayConfig(ctx, k8sClient, testGwConfig, nil)
			createOrUpdateGateway(ctx, k8sClient, testGw, nil)
			createOrUpdateUDPRoute(ctx, k8sClient, testUDPRoute, nil)
			createOrUpdateService(ctx, k8sClient, testSvc, nil)
			createOrUpdateEndpointSlice(ctx, k8sClient, testEndpointSlice, nil)
			// envtest has no nodes; without one the node controller never reports and the
			// startup gate could only open by its timeout
			createOrUpdateNode(ctx, k8sClient, testNode, nil)
			current := &stnrgwv1.Dataplane{ObjectMeta: metav1.ObjectMeta{
				Name: testDataplane.GetName(),
			}}
			_, err := ctrlutil.CreateOrUpdate(ctx, k8sClient, current, func() error {
				testDataplane.Spec.DeepCopyInto(&current.Spec)
				return nil
			})
			Expect(err).Should(Succeed())
		})

		It("should stand by while another replica holds the Lease", func() {
			now := metav1.NewMicroTime(time.Now())
			lease = &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opdefault.DefaultLeaderElectionID,
					Namespace: testNs.GetName(),
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptr.To("another-replica_00000000"),
					LeaseDurationSeconds: ptr.To(int32(300)),
					AcquireTime:          &now,
					RenewTime:            &now,
					LeaseTransitions:     ptr.To(int32(0)),
				},
			}
			Expect(k8sClient.Create(ctx, lease)).Should(Succeed())

			initOperator(ctx, leaderElection)
			op.SetFinalizer(false)
			cdsAddr = cdsServerAddr

			// the config discovery port stays closed and nothing is rendered
			Consistently(cdsPortOpen, 2*time.Second, 100*time.Millisecond).Should(BeFalse(),
				"a standby must not accept config discovery clients")
			// the CRD defaults both Gateway conditions to Unknown; only a rendering leader
			// settles them
			gw := &gwapiv1.Gateway{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testGw), gw)).Should(Succeed())
			for _, c := range gw.Status.Conditions {
				Expect(c.Status).To(Equal(metav1.ConditionUnknown), "a standby must not render")
			}
			dp := &appv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNs.GetName(),
				Name: testGw.GetName()}, dp)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "a standby must not render")
		})

		It("should take over once the Lease is released", func() {
			Expect(k8sClient.Delete(ctx, lease)).Should(Succeed())
			Eventually(cdsPortOpen, timeout, interval).Should(BeTrue())

			clientCtx, clientCancel = context.WithCancel(context.Background())
			ch = make(chan *stnrapiv1.StunnerConfig, 128)
			cl, err := cdsclient.New(cdsServerAddr, "testnamespace/gateway-1", "",
				logger.NewLoggerFactory(stunnerLogLevel))
			Expect(err).Should(Succeed())
			Expect(cl.Watch(clientCtx, ch, false)).Should(Succeed())

			// the first config served is already complete: the startup gate held the
			// render until every controller reported
			first := receive(timeout)
			Expect(first).NotTo(BeNil(), "no config served by the new leader")
			Expect(complete(first)).To(BeTrue(), "the first config is partial: %s", first.String())

			// the Deployment for the Gateway exists now
			Eventually(func() bool {
				dp := &appv1.Deployment{}
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: testNs.GetName(),
					Name: testGw.GetName()}, dp) == nil
			}, timeout, interval).Should(BeTrue())

			// settle: keep the newest config, if any more arrive
			lastConfig = first
			for c := receive(time.Second); c != nil; c = receive(time.Second) {
				lastConfig = c
			}
		})

		It("should serve the same config, once, after a restart", func() {
			op.Stabilize()
			ctrl.Log.Info("stopping the operator")
			cancel()
			Eventually(cdsPortOpen, timeout, interval).Should(BeFalse())

			// the successor serves at the same address, like a replica behind a Service, and
			// starts with empty stores, like a fresh process
			resetStores()
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx, leaderElection, func(c *app.Config) {
				c.CDSAddress, c.CDSAdvertisedAddress = cdsAddr, cdsAddr
				cdsServerAddr = cdsAddr
			})
			op.SetFinalizer(false)
			Eventually(cdsPortOpen, timeout, interval).Should(BeTrue())

			// the watcher reconnects on its own and receives the config exactly once
			got := receive(timeout)
			Expect(got).NotTo(BeNil(), "no config served after the restart")
			Expect(got.DeepEqual(lastConfig)).To(BeTrue(),
				"the restarted operator served a different config:\nbefore: %s\nafter:  %s",
				lastConfig.String(), got.String())
			Expect(receive(2*time.Second)).To(BeNil(), "the restarted operator pushed more than once")
		})

		It("should survive a full cleanup", func() {
			clientCancel()
			clientCancel = nil
			Expect(k8sClient.Delete(ctx, testGwClass)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, testGwConfig)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, testGw)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, testUDPRoute)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, testSvc)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, testEndpointSlice)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, testDataplane)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, testNode)).Should(Succeed())
			op.Stabilize()
			cancel()
		})
	})
}
