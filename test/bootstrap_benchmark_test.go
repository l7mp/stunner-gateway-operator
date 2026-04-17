package integration

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	licensemgr "github.com/l7mp/stunner-gateway-operator/internal/licensemanager"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

const (
	comboReadyTimout       = 2 * time.Minute
	benchmarkClassName     = "bench-gwclass"
	benchmarkConfigName    = "bench-gwconfig"
	benchmarkControlNs     = "bench-control"
	benchmarkNsPrefix      = "bench-ns-"
	benchmarkGatewayPrefix = "bench-gw-"
	benchmarkRoutePrefix   = "bench-udproute-"
)

type comboReadyState struct {
	gw bool
	ur bool
}

type statusWatcher struct {
	mu      sync.RWMutex
	gcReady bool
	states  map[int]comboReadyState
	signalC chan struct{}
}

func newStatusWatcher() *statusWatcher {
	return &statusWatcher{
		states:  map[int]comboReadyState{},
		signalC: make(chan struct{}, 1),
	}
}

func (w *statusWatcher) markGatewayClass(gc *gwapiv1.GatewayClass) {
	if gc.Name != benchmarkClassName {
		return
	}

	ready := conditionTrue(gc.Status.Conditions, string(gwapiv1.GatewayClassConditionStatusAccepted))
	w.mu.Lock()
	w.gcReady = ready
	w.mu.Unlock()
	w.notify()
}

func (w *statusWatcher) markGateway(gw *gwapiv1.Gateway) {
	idx, ok := parseIndexedName(gw.Namespace, benchmarkNsPrefix)
	if !ok {
		return
	}

	ready := gatewayReady(gw)

	w.mu.Lock()
	s := w.states[idx]
	s.gw = ready
	w.states[idx] = s
	w.mu.Unlock()
	w.notify()
}

func (w *statusWatcher) markUDPRoute(ur *stnrgwv1.UDPRoute) {
	idx, ok := parseIndexedName(ur.Namespace, benchmarkNsPrefix)
	if !ok {
		return
	}

	ready := udpRouteReady(ur)
	w.mu.Lock()
	s := w.states[idx]
	s.ur = ready
	w.states[idx] = s
	w.mu.Unlock()
	w.notify()
}

func (w *statusWatcher) waitReady(ctx context.Context, idx int, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		if w.isReady(idx) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			w.mu.RLock()
			gcReady := w.gcReady
			state := w.states[idx]
			w.mu.RUnlock()
			return fmt.Errorf("timed out waiting for combo %d readiness: gc=%v, gw=%v, ur=%v", idx, gcReady, state.gw, state.ur)
		case <-w.signalC:
		}
	}
}

func (w *statusWatcher) isReady(idx int) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	s := w.states[idx]
	return w.gcReady && s.gw && s.ur
}

func (w *statusWatcher) notify() {
	select {
	case w.signalC <- struct{}{}:
	default:
	}
}

func conditionTrue(conds []metav1.Condition, t string) bool {
	c := meta.FindStatusCondition(conds, t)
	return c != nil && c.Status == metav1.ConditionTrue
}

func gatewayReady(gw *gwapiv1.Gateway) bool {
	if !conditionTrue(gw.Status.Conditions, string(gwapiv1.GatewayConditionAccepted)) {
		return false
	}

	p := meta.FindStatusCondition(gw.Status.Conditions, string(gwapiv1.GatewayConditionProgrammed))
	if p == nil {
		return false
	}

	return p.Status == metav1.ConditionTrue || p.Reason == string(gwapiv1.GatewayReasonAddressNotAssigned)
}

func udpRouteReady(ur *stnrgwv1.UDPRoute) bool {
	// Safety guard requested during planning: exactly one attached Gateway parent.
	if len(ur.Status.Parents) != 1 {
		return false
	}

	ps := ur.Status.Parents[0]
	return conditionTrue(ps.Conditions, string(gwapiv1.RouteConditionAccepted)) &&
		conditionTrue(ps.Conditions, string(gwapiv1.RouteConditionResolvedRefs))
}

func parseIndexedName(raw, prefix string) (int, bool) {
	if !strings.HasPrefix(raw, prefix) {
		return 0, false
	}
	v := strings.TrimPrefix(raw, prefix)
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return i, true
}

type benchmarkSystem struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mgrStop context.CancelFunc
	testEnv *envtest.Environment
	cfg     *rest.Config
	scheme  *runtime.Scheme
	client  client.Client
	mgr     ctrl.Manager
	op      *operator.Operator
}

func BenchmarkBootstrap(b *testing.B) {
	runBootstrapBenchmark(b)
}

func runBootstrapBenchmark(b *testing.B) {
	b.Helper()

	oldThrottleTimeout := config.ThrottleTimeout
	oldDataplaneMode := config.DataplaneMode
	oldEndpointSliceAvailable := config.EndpointSliceAvailable

	config.ThrottleTimeout = 10 * time.Millisecond
	config.DataplaneMode = config.DataplaneModeManaged
	config.EndpointSliceAvailable = true

	defer func() {
		config.ThrottleTimeout = oldThrottleTimeout
		config.DataplaneMode = oldDataplaneMode
		config.EndpointSliceAvailable = oldEndpointSliceAvailable
	}()

	sys := setupBenchmarkSystem(b)
	defer teardownBenchmarkSystem(b, sys)

	watcher := newStatusWatcher()
	registerStatusWatchers(b, sys, watcher)

	b.ResetTimer()
	startAll := time.Now()
	var totalCombo time.Duration
	iterDurations := make([]time.Duration, b.N)

	for i := 0; i < b.N; i++ {
		comboStart := time.Now()
		if err := createCombo(sys.ctx, sys.client, i); err != nil {
			b.Fatalf("failed to create combo %d: %v", i, err)
		}

		if err := watcher.waitReady(sys.ctx, i, comboReadyTimout); err != nil {
			b.Fatalf("combo %d did not converge: %v (%s)", i, err, describeComboStatuses(sys.ctx, sys.client, i))
		}

		elapsed := time.Since(comboStart)
		iterDurations[i] = elapsed
		totalCombo += elapsed
	}

	total := time.Since(startAll)
	b.StopTimer()

	for i, d := range iterDurations {
		fmt.Printf("BenchmarkBootstrap iteration=%02d duration=%s\n", i+1, d.Round(time.Millisecond))
	}

	if b.N > 0 {
		b.ReportMetric(float64(totalCombo.Milliseconds())/float64(b.N), "ms/combo")
	}
	b.ReportMetric(float64(total.Milliseconds()), "ms/bootstrap")
}

func setupBenchmarkSystem(b *testing.B) *benchmarkSystem {
	b.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	scheme := runtime.NewScheme()
	mustAddScheme(b, scheme)

	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join("..", "config", "gateway-api-v1.0.0", "crd"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		cancel()
		b.Fatalf("failed to start envtest: %v", err)
	}

	on := true
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:     scheme,
		Controller: ctrlcfg.Controller{SkipNameValidation: &on},
	})
	if err != nil {
		cancel()
		_ = testEnv.Stop()
		b.Fatalf("failed to create manager: %v", err)
	}

	logger := logr.Discard()
	ctrl.SetLogger(logger)
	lic := licensemgr.NewManager("", logger)

	r := renderer.NewRenderer(renderer.RendererConfig{
		Scheme:         scheme,
		LicenseManager: lic,
		Logger:         logger,
	})

	u := updater.NewUpdater(updater.UpdaterConfig{
		Manager: mgr,
		Logger:  logger,
	})

	cdsAddr := reserveLoopbackAddress(b)
	cds := config.NewCDSServer(cdsAddr, logger)

	op := operator.NewOperator(operator.OperatorConfig{
		ControllerName: opdefault.DefaultControllerName,
		Manager:        mgr,
		RenderCh:       r.GetRenderChannel(),
		ConfigCh:       cds.GetConfigUpdateChannel(),
		UpdaterCh:      u.GetUpdaterChannel(),
		Logger:         logger,
	})

	lic.SetOperatorChannel(op.GetOperatorChannel())
	r.SetOperatorChannel(op.GetOperatorChannel())
	u.SetAckChannel(op.GetOperatorChannel())
	op.SetProgressReporters(r, u, cds)

	if err := r.Start(ctx); err != nil {
		cancel()
		_ = testEnv.Stop()
		b.Fatalf("failed to start renderer: %v", err)
	}

	if err := u.Start(ctx); err != nil {
		cancel()
		_ = testEnv.Stop()
		b.Fatalf("failed to start updater: %v", err)
	}

	if err := cds.Start(ctx); err != nil {
		cancel()
		_ = testEnv.Stop()
		b.Fatalf("failed to start cds server: %v", err)
	}

	mgrCtx, mgrStop := context.WithCancel(context.Background())
	go func() {
		_ = mgr.Start(mgrCtx)
	}()

	if err := op.Start(ctx, nil); err != nil {
		mgrStop()
		cancel()
		_ = testEnv.Stop()
		b.Fatalf("failed to start operator: %v", err)
	}

	if !mgr.GetCache().WaitForCacheSync(ctx) {
		mgrStop()
		cancel()
		_ = testEnv.Stop()
		b.Fatalf("manager cache did not sync")
	}

	client := mgr.GetClient()
	if err := createSharedObjects(ctx, client); err != nil {
		mgrStop()
		cancel()
		_ = testEnv.Stop()
		b.Fatalf("failed to create shared objects: %v", err)
	}

	return &benchmarkSystem{
		ctx:     ctx,
		cancel:  cancel,
		mgrStop: mgrStop,
		testEnv: testEnv,
		cfg:     cfg,
		scheme:  scheme,
		client:  client,
		mgr:     mgr,
		op:      op,
	}
}

func teardownBenchmarkSystem(b *testing.B, sys *benchmarkSystem) {
	b.Helper()

	sys.op.Stabilize()
	sys.cancel()
	sys.mgrStop()

	if err := sys.testEnv.Stop(); err != nil {
		b.Fatalf("failed to stop envtest: %v", err)
	}
}

func registerStatusWatchers(b *testing.B, sys *benchmarkSystem, watcher *statusWatcher) {
	b.Helper()

	gcInf, err := sys.mgr.GetCache().GetInformer(sys.ctx, &gwapiv1.GatewayClass{})
	if err != nil {
		b.Fatalf("failed to get GatewayClass informer: %v", err)
	}

	gwInf, err := sys.mgr.GetCache().GetInformer(sys.ctx, &gwapiv1.Gateway{})
	if err != nil {
		b.Fatalf("failed to get Gateway informer: %v", err)
	}

	urInf, err := sys.mgr.GetCache().GetInformer(sys.ctx, &stnrgwv1.UDPRoute{})
	if err != nil {
		b.Fatalf("failed to get UDPRoute informer: %v", err)
	}

	_, err = gcInf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if gc, ok := obj.(*gwapiv1.GatewayClass); ok {
				watcher.markGatewayClass(gc)
			}
		},
		UpdateFunc: func(_, newObj any) {
			if gc, ok := newObj.(*gwapiv1.GatewayClass); ok {
				watcher.markGatewayClass(gc)
			}
		},
	})
	if err != nil {
		b.Fatalf("failed to add GatewayClass handler: %v", err)
	}

	_, err = gwInf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if gw, ok := obj.(*gwapiv1.Gateway); ok {
				watcher.markGateway(gw)
			}
		},
		UpdateFunc: func(_, newObj any) {
			if gw, ok := newObj.(*gwapiv1.Gateway); ok {
				watcher.markGateway(gw)
			}
		},
	})
	if err != nil {
		b.Fatalf("failed to add Gateway handler: %v", err)
	}

	_, err = urInf.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if ur, ok := obj.(*stnrgwv1.UDPRoute); ok {
				watcher.markUDPRoute(ur)
			}
		},
		UpdateFunc: func(_, newObj any) {
			if ur, ok := newObj.(*stnrgwv1.UDPRoute); ok {
				watcher.markUDPRoute(ur)
			}
		},
	})
	if err != nil {
		b.Fatalf("failed to add UDPRoute handler: %v", err)
	}
}

func createSharedObjects(ctx context.Context, c client.Client) error {
	ns := testutils.TestNs.DeepCopy()
	ns.Name = benchmarkControlNs

	gwConf := testutils.TestGwConfig.DeepCopy()
	gwConf.Name = benchmarkConfigName
	gwConf.Namespace = benchmarkControlNs

	gwc := testutils.TestGwClass.DeepCopy()
	gwc.Name = benchmarkClassName
	gwc.Namespace = ""
	gwc.Spec.ParametersRef.Name = benchmarkConfigName
	controlNs := gwapiv1.Namespace(benchmarkControlNs)
	gwc.Spec.ParametersRef.Namespace = &controlNs

	dp := testutils.TestDataplane.DeepCopy()

	objs := []client.Object{ns, gwConf, gwc, dp}
	for _, obj := range objs {
		if err := c.Create(ctx, obj); err != nil {
			return err
		}
	}

	return nil
}

func createCombo(ctx context.Context, c client.Client, idx int) error {
	ns := testutils.TestNs.DeepCopy()
	ns.Name = fmt.Sprintf("%s%d", benchmarkNsPrefix, idx)

	gw := testutils.TestGw.DeepCopy()
	gw.Name = fmt.Sprintf("%s%d", benchmarkGatewayPrefix, idx)
	gw.Namespace = ns.Name
	gw.Spec.GatewayClassName = gwapiv1.ObjectName(benchmarkClassName)
	gw.Spec.Listeners = []gwapiv1.Listener{gw.Spec.Listeners[0]}

	staticSvc := testutils.TestStaticSvc.DeepCopy()
	staticSvc.Name = fmt.Sprintf("bench-static-%d", idx)
	staticSvc.Namespace = ns.Name

	ur := testutils.TestUDPRoute.DeepCopy()
	ur.Name = fmt.Sprintf("%s%d", benchmarkRoutePrefix, idx)
	ur.Namespace = ns.Name
	ur.Spec.ParentRefs[0].Name = gwapiv1.ObjectName(gw.Name)
	k := gwapiv1.Kind("StaticService")
	g := gwapiv1.Group(stnrgwv1.GroupVersion.Group)
	ur.Spec.Rules[0].BackendRefs[0].Group = &g
	ur.Spec.Rules[0].BackendRefs[0].Kind = &k
	ur.Spec.Rules[0].BackendRefs[0].Name = gwapiv1.ObjectName(staticSvc.Name)
	ur.Spec.Rules[0].BackendRefs[0].Port = nil

	objects := []client.Object{ns, gw, staticSvc, ur}
	for _, obj := range objects {
		if err := c.Create(ctx, obj); err != nil {
			return err
		}
	}

	return nil
}

func describeComboStatuses(ctx context.Context, c client.Client, idx int) string {
	ns := fmt.Sprintf("%s%d", benchmarkNsPrefix, idx)

	gc := &gwapiv1.GatewayClass{}
	gcErr := c.Get(ctx, client.ObjectKey{Name: benchmarkClassName}, gc)

	gw := &gwapiv1.Gateway{}
	gwErr := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: fmt.Sprintf("%s%d", benchmarkGatewayPrefix, idx)}, gw)

	ur := &stnrgwv1.UDPRoute{}
	urErr := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: fmt.Sprintf("%s%d", benchmarkRoutePrefix, idx)}, ur)

	b := strings.Builder{}
	b.WriteString("gc=")
	if gcErr != nil {
		b.WriteString(gcErr.Error())
	} else {
		b.WriteString(conditionSummary(gc.Status.Conditions))
	}

	b.WriteString("; gw=")
	if gwErr != nil {
		b.WriteString(gwErr.Error())
	} else {
		b.WriteString(conditionSummary(gw.Status.Conditions))
	}

	b.WriteString("; ur=")
	if urErr != nil {
		b.WriteString(urErr.Error())
	} else if len(ur.Status.Parents) == 0 {
		b.WriteString("no-parents")
	} else {
		b.WriteString(conditionSummary(ur.Status.Parents[0].Conditions))
	}

	return b.String()
}

func conditionSummary(conds []metav1.Condition) string {
	if len(conds) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(conds))
	for i := range conds {
		parts = append(parts, fmt.Sprintf("%s=%s/%s", conds[i].Type, conds[i].Status, conds[i].Reason))
	}

	return strings.Join(parts, ",")
}

func reserveLoopbackAddress(b *testing.B) string {
	b.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to reserve loopback address: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		b.Fatalf("failed to release temporary listener: %v", err)
	}

	return addr
}

func mustAddScheme(b *testing.B, scheme *runtime.Scheme) {
	b.Helper()

	if err := clientgoscheme.AddToScheme(scheme); err != nil { //nolint:staticcheck
		b.Fatalf("failed to add client-go scheme: %v", err)
	}

	if err := gwapiv1.AddToScheme(scheme); err != nil { //nolint:staticcheck
		b.Fatalf("failed to add gateway v1 scheme: %v", err)
	}

	if err := gwapiv1a2.AddToScheme(scheme); err != nil { //nolint:staticcheck
		b.Fatalf("failed to add gateway v1alpha2 scheme: %v", err)
	}

	if err := stnrgwv1.AddToScheme(scheme); err != nil { //nolint:staticcheck
		b.Fatalf("failed to add stunner gateway scheme: %v", err)
	}

	if err := appv1.AddToScheme(scheme); err != nil { //nolint:staticcheck
		b.Fatalf("failed to add apps scheme: %v", err)
	}

	if err := corev1.AddToScheme(scheme); err != nil { //nolint:staticcheck
		b.Fatalf("failed to add core scheme: %v", err)
	}

	if err := discoveryv1.AddToScheme(scheme); err != nil { //nolint:staticcheck
		b.Fatalf("failed to add discovery scheme: %v", err)
	}
}
