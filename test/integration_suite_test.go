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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	// "k8s.io/client-go/kubernetes/scheme"

	// "github.com/go-logr/logr"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	// logf "sigs.k8s.io/controller-runtime/pkg/log"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var _ = fmt.Sprintf("%d", 1)

func init() {
	os.Setenv("ACK_GINKGO_DEPRECATIONS", "1.16.5")
}

// Define utility constants for object names and testing timeouts/durations and intervals.
const (
	timeout = time.Second * 10
	// duration = time.Second * 10
	interval = time.Millisecond * 250
	loglevel = -4
	//loglevel = -1
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	scheme    *runtime.Scheme = runtime.NewScheme()
	// zapCfg                    = zap.Config{
	// 	Encoding:    "console",
	// 	OutputPaths: []string{"stderr"},
	// 	EncoderConfig: zapcore.EncoderConfig{
	// 		MessageKey:  "message",
	// 		TimeKey:     "time",
	// 		EncodeTime:  zapcore.ISO8601TimeEncoder,
	// 		LevelKey:    "level",
	// 		EncodeLevel: zapcore.CapitalLevelEncoder,
	// 	},
	// }
)

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

var _ = BeforeSuite(func() {
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), func(o *zap.Options) {
		o.TimeEncoder = zapcore.RFC3339NanoTimeEncoder
	}, zap.Level(zapcore.Level(loglevel))))
	setupLog := ctrl.Log.WithName("setup")

	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join("..", "config", "gateway-api-v0.6.2", "crd"),
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
	err = gwapiv1a2.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())
	// err = gwapiv1b1.AddToScheme(scheme)
	// Expect(err).NotTo(HaveOccurred())

	// STUNner CRD scheme
	err = stnrgwv1a1.AddToScheme(scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	setupLog.Info("creating a testing namespace")
	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: string(testutils.TestNs),
		},
	})).Should(Succeed())

	setupLog.Info("setting up client manager")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
		Port:               9443,
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

	// make rendering fast!
	config.ThrottleTimeout = time.Millisecond

	setupLog.Info("setting up operator")
	op := operator.NewOperator(operator.OperatorConfig{
		ControllerName: opdefault.DefaultControllerName,
		Manager:        mgr,
		RenderCh:       r.GetRenderChannel(),
		UpdaterCh:      u.GetUpdaterChannel(),
		Logger:         ctrl.Log,
	})

	r.SetOperatorChannel(op.GetOperatorChannel())

	setupLog.Info("starting renderer thread")
	err = r.Start(ctx)
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("starting updater thread")
	err = u.Start(ctx)
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("starting operator thread")
	err = op.Start(ctx)
	Expect(err).NotTo(HaveOccurred())

	setupLog.Info("starting manager")
	// must be explicitly cancelled!
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

}, 60)

var _ = AfterSuite(func() {
	By("removing test namespace")
	Expect(k8sClient.Delete(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: string(testutils.TestNs),
		},
	})).Should(Succeed())

	cancel()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

type UDPRouteMutator func(current *gwapiv1a2.UDPRoute)

func recreateOrUpdateUDPRoute(f UDPRouteMutator) {
	current := &gwapiv1a2.UDPRoute{ObjectMeta: metav1.ObjectMeta{
		Name:      testUDPRoute.GetName(),
		Namespace: testUDPRoute.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		testutils.TestUDPRoute.Spec.DeepCopyInto(&current.Spec)
		f(current)
		return nil
	})
	Expect(err).Should(Succeed())
}

type GatewayMutator func(current *gwapiv1a2.Gateway)

func recreateOrUpdateGateway(f GatewayMutator) {
	current := &gwapiv1a2.Gateway{ObjectMeta: metav1.ObjectMeta{
		Name:      testGw.GetName(),
		Namespace: testGw.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		testutils.TestGw.Spec.DeepCopyInto(&current.Spec)
		f(current)
		return nil
	})
	Expect(err).Should(Succeed())
}

type GatewayConfigMutator func(current *stnrgwv1a1.GatewayConfig)

func recreateOrUpdateGatewayConfig(f GatewayConfigMutator) {
	current := &stnrgwv1a1.GatewayConfig{ObjectMeta: metav1.ObjectMeta{
		Name:      testGwConfig.GetName(),
		Namespace: testGwConfig.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		testutils.TestGwConfig.Spec.DeepCopyInto(&current.Spec)
		f(current)
		return nil
	})
	Expect(err).Should(Succeed())
}

type SecretMutator func(current *corev1.Secret)

func recreateOrUpdateSecret(f SecretMutator) {
	current := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      testSecret.GetName(),
		Namespace: testSecret.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.Type = testSecret.Type
		current.Data = make(map[string][]byte)
		for k, v := range testSecret.Data {
			current.Data[k] = v
		}
		f(current)
		return nil
	})
	Expect(err).Should(Succeed())
}

func recreateOrUpdateAuthSecret(f SecretMutator) {
	current := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      testAuthSecret.GetName(),
		Namespace: testAuthSecret.GetNamespace(),
	}}

	_, err := createOrUpdate(ctx, k8sClient, current, func() error {
		current.Type = testAuthSecret.Type
		current.Data = make(map[string][]byte)
		for k, v := range testAuthSecret.Data {
			current.Data[k] = v
		}
		f(current)
		return nil
	})
	Expect(err).Should(Succeed())
}

type NodeMutator func(current *corev1.Node)

func statusUpdateNode(name string, f NodeMutator) {
	current := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: name,
	}}

	err := k8sClient.Get(ctx, client.ObjectKeyFromObject(current), current)
	Expect(err).Should(Succeed())

	f(current)

	err = k8sClient.Status().Update(ctx, current)
	Expect(err).Should(Succeed())
}

// createOrUpdate will retry when ctrlutil.CreateOrUpdate fails. This is useful to robustify tests:
// sometimes an update is going on why we try to run the next test and then the CreateOrUpdate may
// fail with a Conflict.
func createOrUpdate(ctx context.Context, c client.Client, obj client.Object, f ctrlutil.MutateFn) (ctrlutil.OperationResult, error) {
	res, err := ctrlutil.CreateOrUpdate(ctx, c, obj, f)
	if err == nil {
		return res, err
	}

	for i := 1; i < 5; i++ {
		res, err = ctrlutil.CreateOrUpdate(ctx, c, obj, f)
		if err == nil {
			return res, err
		}
	}

	return res, err
}
