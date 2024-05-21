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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// "k8s.io/client-go/kubernetes/scheme"

	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

var _ = fmt.Sprintf("%d", 1)

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	cdsServerAddr = ":63478"
	newCert64     = "bmV3Y2VydA==" // newcert
	newKey64      = "bmV3a2V5"     // newkey
	timeout       = time.Second * 10
	// duration = time.Second * 10
	interval = time.Millisecond * 250
	loglevel = -4
	//loglevel = -1
	stunnerLogLevel = "all:TRACE"
	//stunnerLogLevel = "all:ERROR"
)

var (
	// Resources
	testNs            *corev1.Namespace
	testGwClass       *gwapiv1.GatewayClass
	testGwConfig      *stnrgwv1.GatewayConfig
	testGw            *gwapiv1.Gateway
	testUDPRouteV1A2  *gwapiv1a2.UDPRoute
	testUDPRoute      *stnrgwv1.UDPRoute
	testSvc           *corev1.Service
	testEndpoint      *corev1.Endpoints
	testEndpointSlice *discoveryv1.EndpointSlice
	testNode          *corev1.Node
	testSecret        *corev1.Secret
	testAuthSecret    *corev1.Secret
	testStaticSvc     *stnrgwv1.StaticService
	testDataplane     *stnrgwv1.Dataplane
	// Globals
	cfg              *rest.Config
	k8sClient        client.Client
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel, opCancel context.CancelFunc
	scheme           *runtime.Scheme = runtime.NewScheme()
	op               *operator.Operator
	setupLog         logr.Logger
)

func init() {
	os.Setenv("ACK_GINKGO_DEPRECATIONS", "1.16.5")
	os.Setenv("ACK_GINKGO_RC", "true")
}

var _ = BeforeSuite(func() {
	opts := zap.Options{
		Development:     true,
		DestWriter:      GinkgoWriter,
		StacktraceLevel: zapcore.Level(3),
		TimeEncoder:     zapcore.RFC3339NanoTimeEncoder,
		Level:           zapcore.Level(loglevel),
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog = ctrl.Log.WithName("setup")

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	InitResources()
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join("..", "config", "gateway-api-v1.0.0", "crd"),
		},
		ErrorIfCRDPathMissing:    true,
		AttachControlPlaneOutput: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// Gateway API schemes
	err = gwapiv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	err = gwapiv1a2.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	// STUNner CRD scheme
	err = stnrgwv1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	setupLog.Info("creating a testing namespace")
	Expect(k8sClient.Create(ctx, testNs)).Should(Succeed())
})

var _ = AfterSuite(func() {
	By("removing test namespace")
	Expect(k8sClient.Delete(ctx, testNs)).Should(Succeed())

	cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func initOperator(mgrCtx, opCtx context.Context) {
	setupLog.Info("setting up client manager")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("setting up STUNner config renderer")
	r := renderer.NewRenderer(renderer.RendererConfig{
		Scheme: scheme,
		Logger: ctrl.Log,
	})
	Expect(r).NotTo(BeNil())

	setupLog.Info("setting up updater client")
	u := updater.NewUpdater(updater.UpdaterConfig{
		Manager: mgr,
		Logger:  ctrl.Log,
	})

	setupLog.Info("setting up CDS server", "address", cdsServerAddr)
	c := config.NewCDSServer(cdsServerAddr, ctrl.Log)

	// make rendering fast!
	config.ThrottleTimeout = 10 * time.Millisecond

	setupLog.Info("setting up operator")
	op = operator.NewOperator(operator.OperatorConfig{
		ControllerName: opdefault.DefaultControllerName,
		Manager:        mgr,
		RenderCh:       r.GetRenderChannel(),
		ConfigCh:       c.GetConfigUpdateChannel(),
		UpdaterCh:      u.GetUpdaterChannel(),
		Logger:         ctrl.Log,
	})

	r.SetOperatorChannel(op.GetOperatorChannel())
	u.SetAckChannel(op.GetOperatorChannel())
	op.SetProgressReporters(r, u, c)

	setupLog.Info("starting renderer thread")
	err = r.Start(mgrCtx)
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("starting updater thread")
	err = u.Start(mgrCtx)
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("starting config discovery server")
	err = c.Start(mgrCtx)
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("starting operator thread")
	err = op.Start(opCtx, nil)
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("starting manager")
	// must be explicitly cancelled!
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(mgrCtx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
}

func InitResources() {
	testNs = testutils.TestNs.DeepCopy()
	testGwClass = testutils.TestGwClass.DeepCopy()
	testGwConfig = testutils.TestGwConfig.DeepCopy()
	testGw = testutils.TestGw.DeepCopy()
	testUDPRoute = testutils.TestUDPRoute.DeepCopy()
	testSvc = testutils.TestSvc.DeepCopy()
	testEndpoint = testutils.TestEndpoint.DeepCopy()
	testEndpointSlice = testutils.TestEndpointSlice.DeepCopy()
	testNode = testutils.TestNode.DeepCopy()
	testSecret = testutils.TestSecret.DeepCopy()
	testAuthSecret = testutils.TestAuthSecret.DeepCopy()
	testStaticSvc = testutils.TestStaticSvc.DeepCopy()
	testDataplane = testutils.TestDataplane.DeepCopy()
	testUDPRouteV1A2 = testutils.TestUDPRouteV1A2.DeepCopy()
}

func TimestampEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format(time.RFC3339Nano))
}

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	// for gingko/v2
	// suiteConfig, reporterConfig := GinkgoConfiguration()
	// reporterConfig.FullTrace = true
	// RunSpecs(t, "Controller Suite", suiteConfig, reporterConfig)

	// RunSpecsWithDefaultAndCustomReporters(t,
	// 	"Controller Suite",
	// 	[]Reporter{envtest.NewlineReporter{}},
	// )

	RunSpecs(t, "Controller Suite")
}

// managed mode test helper
type ConfigChecker func(conf *stnrv1.StunnerConfig) bool

func checkConfig(ch chan *stnrv1.StunnerConfig, checker ConfigChecker) bool {
	timeoutCh := time.After(timeout)
	for {
		select {
		case <-timeoutCh:
			return false
		case c := <-ch:
			// fmt.Printf("--------------------\nCHECKER 0: %#v\n--------------------\n", c)
			if c == nil {
				continue
			}
			ret := checker(c)
			if !ret {
				continue
			}
			return true
		}
	}
}

var _ = Describe("Integration test:", Ordered, func() {
	// Endpoints controller
	// LEGACY
	Context(`When using the "legacy" dataplane mode with the legacy endpoints controller`, Ordered, func() {
		It(`should be possible to initialize the operator`, func() {
			config.DataplaneMode = config.DataplaneModeLegacy
			config.EndpointSliceAvailable = false
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx, ctx)
			op.SetFinalizer(false) // we call the finalizer manually
			InitResources()
		})
	})

	testLegacyModeEndpointController()

	Context(`When terminating the operator after the legacy-mode test with the endpoints controller`, Ordered, func() {
		It("should stabilize", func() {
			op.Stabilize()
			cancel()
		})
	})

	// EndpointSlice controller
	Context(`When using the "legacy" dataplane mode with the legacy endpointslice controller`, func() {
		It(`should be possible to restart the operator using ghe endpointslice controller`, func() {
			config.DataplaneMode = config.DataplaneModeLegacy
			config.EndpointSliceAvailable = true
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx, ctx)
			op.SetFinalizer(false) // we call the finalizer manually
			InitResources()
		})
	})

	testLegacyMode()

	Context(`When terminating the operator after the legacy-mode test with the endpointslice controller`, Ordered, func() {
		It("should stabilize", func() {
			op.Stabilize()
			cancel()
		})
	})

	// MANAGED
	// Endpoints controller
	Context(`When using the "managed" dataplane mode with the legacy endpoints controller`, func() {
		It(`should be possible to set the dataplane mode to "managed"`, func() {
			config.EndpointSliceAvailable = false
			config.DataplaneMode = config.DataplaneModeManaged
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx, ctx)
			op.SetFinalizer(false) // we call the finalizer manually
			InitResources()
		})
	})

	testManagedModeEndpointController()

	Context(`When terminating the operator after the managed-mode test with the endpointslice controller`, Ordered, func() {
		It("should stabilize", func() {
			op.Stabilize()
			cancel()
		})
	})

	// MANAGED
	Context(`When using the "managed" dataplane mode with the legacy endpointslice controller`, func() {
		It(`should be possible to set the dataplane mode to "managed"`, func() {
			config.EndpointSliceAvailable = true
			config.DataplaneMode = config.DataplaneModeManaged
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx, ctx)
			op.SetFinalizer(false) // we call the finalizer manually
			InitResources()
		})
	})

	testManagedMode()

	Context(`When terminating the operator after the managed-mode test with the endpointslice controller`, Ordered, func() {
		It("should stabilize", func() {
			op.Stabilize()
			cancel()
		})
	})

	// FINALIZER
	Context(`When trying to finalize the operator`, Ordered, func() {
		It(`should be possible to set up a new operator`, func() {
			config.EndpointSliceAvailable = true
			config.DataplaneMode = config.DataplaneModeManaged
			ctx, cancel = context.WithCancel(context.Background())
			opCtx, opC := context.WithCancel(context.Background())
			opCancel = opC
			initOperator(ctx, opCtx)
			op.SetFinalizer(true) // should be the default
			InitResources()
			setupLog.Info("opcancel", "cancel", fmt.Sprintf("%#v", opCancel))
		})
	})

	testFinalizer()

	Context(`When terminating the operator`, Ordered, func() {
		It("should stabilize", func() {
			op.Stabilize()
			// AfterSuite calls cancel()
		})
	})

})
