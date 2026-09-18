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
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap/zapcore"
	appv1 "k8s.io/api/apps/v1"
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

	"github.com/l7mp/stunner-gateway-operator/internal/app"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

// auxiliary tests
var auxiliaryTest = func() {}

// Empty customer key for testing
var customerTestKey string

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	newCert64 = "bmV3Y2VydA==" // newcert
	newKey64  = "bmV3a2V5"     // newkey
	timeout   = time.Second * 10
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
	testTCPRoute      *stnrgwv1.TCPRoute
	testTCPRouteV1    *gwapiv1.TCPRoute
	testSvc           *corev1.Service
	testEndpoint      *corev1.Endpoints
	testEndpointSlice *discoveryv1.EndpointSlice
	testNode          *corev1.Node
	testSecret        *corev1.Secret
	testAuthSecret    *corev1.Secret
	testStaticSvc     *stnrgwv1.StaticService
	testDataplane     *stnrgwv1.Dataplane
	testDaemonSet     *appv1.DaemonSet

	// Globals
	cfg              *rest.Config
	k8sClient        client.Client
	testEnv          *envtest.Environment
	ctx              context.Context
	cancel, opCancel context.CancelFunc
	scheme           *runtime.Scheme = runtime.NewScheme()
	op               *operator.Operator
	theApp           *app.App
	cdsServerAddr    string
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
			filepath.Join("..", "config", "gateway-api-v1.6.2", "crd"),
		},
		ErrorIfCRDPathMissing:    true,
		AttachControlPlaneOutput: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = clientgoscheme.AddToScheme(scheme) //nolint:staticcheck
	Expect(err).NotTo(HaveOccurred())

	// Gateway API schemes
	err = gwapiv1.AddToScheme(scheme) //nolint:staticcheck
	Expect(err).NotTo(HaveOccurred())
	err = gwapiv1a2.AddToScheme(scheme) //nolint:staticcheck
	Expect(err).NotTo(HaveOccurred())

	// STUNner CRD scheme
	err = stnrgwv1.AddToScheme(scheme) //nolint:staticcheck
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	setupLog.Info("creating a testing namespace")
	Expect(k8sClient.Create(ctx, testNs)).Should(Succeed())
})

var _ = AfterSuite(func() {
	By("removing test namespace")
	// the last phase may have cancelled the shared context already
	Expect(k8sClient.Delete(context.Background(), testNs)).Should(Succeed())

	cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// initOperator assembles and starts a fresh operator instance on the current global mode
// settings, the way main does through the app package. tweak may adjust the configuration
// before assembly.
func initOperator(ctx context.Context, tweak ...func(*app.Config)) {
	appCfg := app.NewConfig()
	appCfg.DataplaneMode = config.DataplaneMode.String()
	appCfg.DisableEndpointSliceController = !config.EndpointSliceAvailable
	// make rendering fast, and do not hold the first render for controllers with nothing to
	// reconcile
	appCfg.ThrottleTimeout = 10 * time.Millisecond
	appCfg.StartupRenderTimeout = 500 * time.Millisecond
	appCfg.MetricsAddr, appCfg.ProbeAddr, appCfg.PprofAddr = "0", "0", "0"
	appCfg.SkipNameValidation = true
	appCfg.CustomerKey = customerTestKey

	// let the kernel pick a free port: a blind random port may collide with the ephemeral
	// sockets of a previous test phase
	cdsProbe, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	cdsPort := cdsProbe.Addr().(*net.TCPAddr).Port
	Expect(cdsProbe.Close()).To(Succeed())
	appCfg.CDSAddress = fmt.Sprintf(":%d", cdsPort)
	appCfg.CDSAdvertisedAddress = fmt.Sprintf("127.0.0.1:%d", cdsPort)
	cdsServerAddr = appCfg.CDSAdvertisedAddress

	for _, t := range tweak {
		t(&appCfg)
	}

	setupLog.Info("setting up the operator", appCfg.Summary()...)
	a, err := app.New(appCfg, cfg, scheme, ctrl.Log)
	Expect(err).NotTo(HaveOccurred())
	op = a.Operator
	theApp = a

	setupLog.Info("starting the operator")
	// must be explicitly cancelled!
	go func() {
		defer GinkgoRecover()
		err := a.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run the operator")
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
	testTCPRoute = testutils.TestTCPRoute.DeepCopy()
	testTCPRouteV1 = testutils.TestTCPRouteV1.DeepCopy()
	testDaemonSet = testutils.TestDaemonSet.DeepCopy()
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
type ConfigChecker func(conf *stnrapiv1.StunnerConfig) bool

func checkConfig(ch chan *stnrapiv1.StunnerConfig, checker ConfigChecker) bool {
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

// -----------------------
// Test cases
// -----------------------
var _ = Describe("Integration test:", Ordered, func() {
	// LEGACY
	// Endpoints controller
	legacyModeEndpointControllerTest()

	// LEGACY
	// EndpointSlice controller
	legacyModeTest()

	// MANAGED
	// Endpoints controller
	managedModeEndpointControllerTest()

	// MANAGED
	// EndpointSlice controller
	managedModeTest()

	// HA: standby and restart
	haOperatorTest()

	// Auxiliary
	auxiliaryTest()

	// MANAGED
	// FINALIZER
	finalizerTest()
})

func legacyModeEndpointControllerTest() {
	Context(`When using the "legacy" dataplane mode with the legacy endpoints controller`, Ordered, func() {
		It(`should be possible to initialize the operator`, func() {
			config.DataplaneMode = config.DataplaneModeLegacy
			config.EndpointSliceAvailable = false
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx)
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
}

func legacyModeTest() {
	Context(`When using the "legacy" dataplane mode with the legacy endpointslice controller`, func() {
		It(`should be possible to restart the operator using ghe endpointslice controller`, func() {
			config.DataplaneMode = config.DataplaneModeLegacy
			config.EndpointSliceAvailable = true
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx)
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
}

func managedModeEndpointControllerTest() {
	Context(`When using the "managed" dataplane mode with the legacy endpoints controller`, func() {
		It(`should be possible to set the dataplane mode to "managed"`, func() {
			config.EndpointSliceAvailable = false
			config.DataplaneMode = config.DataplaneModeManaged
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx)
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
}

func managedModeTest() {
	Context(`When using the "managed" dataplane mode with the legacy endpointslice controller`, func() {
		It(`should be possible to set the dataplane mode to "managed"`, func() {
			config.EndpointSliceAvailable = true
			config.DataplaneMode = config.DataplaneModeManaged
			ctx, cancel = context.WithCancel(context.Background())
			initOperator(ctx)
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
}

func finalizerTest() {
	Context(`When trying to finalize the operator`, Ordered, func() {
		It(`should be possible to set up a new operator`, func() {
			config.EndpointSliceAvailable = true
			config.DataplaneMode = config.DataplaneModeManaged
			ctx, cancel = context.WithCancel(context.Background())
			// the operator gets its own context: stopping it runs the finalizer on the way
			// out, while the checks keep using ctx for their API calls
			opCtx, opC := context.WithCancel(context.Background())
			opCancel = opC
			initOperator(opCtx)
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
}
