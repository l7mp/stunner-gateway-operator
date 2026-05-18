package metrics

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// LoopHeartbeatInterval is how often each worker goroutine bumps its heartbeat
// gauge during idle periods. Picked low enough to give plenty of margin under
// LoopStalenessThreshold while remaining negligible overhead.
const LoopHeartbeatInterval = 5 * time.Second

// LoopStalenessThreshold is the maximum acceptable age of a worker goroutine's
// last heartbeat before the /healthz check considers the loop hung. The
// heartbeat is bumped on every select wakeup, including at the start of the
// work branch — so during a single long-running iteration (e.g. a slow
// ProcessUpdate) the gauge does not refresh until that iteration completes.
// Existing duration histograms cap at 60s, so this threshold is set well above
// that to avoid restarting pods that are slow rather than wedged.
const LoopStalenessThreshold = 2 * time.Minute

// reconcileTimeBuckets matches the bucket boundaries used by controller-runtime for
// controller_runtime_reconcile_time_seconds, so that all operator histograms are
// comparable without re-binning.
var reconcileTimeBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5,
	0.6, 0.7, 0.8, 0.9, 1.0, 1.25, 1.5, 1.75, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5,
	5, 6, 7, 8, 9, 10, 15, 20, 25, 30, 40, 50, 60,
}

var (
	// RenderTotal is the total number of render cycles executed by the renderer thread.
	RenderTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "stunner_gateway_operator_render_total",
		Help: "Total number of render cycles executed by the renderer thread.",
	})

	// RenderDuration tracks the duration of each render cycle.
	RenderDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                            "stunner_gateway_operator_render_time_seconds",
		Help:                            "Duration of render cycles executed by the renderer thread.",
		Buckets:                         reconcileTimeBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	})

	// UpdateTotal is the total number of update cycles executed by the updater thread,
	// split by result ("success" or "error").
	UpdateTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stunner_gateway_operator_update_total",
		Help: "Total number of update cycles executed by the updater thread.",
	}, []string{"result"})

	// UpdateErrors is the total number of update cycles that returned an error.
	UpdateErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "stunner_gateway_operator_update_errors_total",
		Help: "Total number of update cycles that returned an error.",
	})

	// UpdateDuration tracks the duration of each update cycle.
	UpdateDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:                            "stunner_gateway_operator_update_time_seconds",
		Help:                            "Duration of update cycles executed by the updater thread.",
		Buckets:                         reconcileTimeBuckets,
		NativeHistogramBucketFactor:     1.1,
		NativeHistogramMaxBucketNumber:  100,
		NativeHistogramMinResetDuration: 1 * time.Hour,
	})

	// ResourceOperationsTotal counts individual Kubernetes API operations performed by
	// the updater, labelled by scope ("spec" or "status"), resource kind, and operation
	// (attempt, created, updated, error, suppressed, …).
	ResourceOperationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stunner_gateway_operator_resource_operations_total",
		Help: "Total number of Kubernetes API operations performed by the updater thread.",
	}, []string{"scope", "kind", "operation"})

	// ReconcileEventsTotal counts reconcile events received by the operator event loop,
	// split by result ("passed" when a render is triggered, "throttled" when rate-limited).
	ReconcileEventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "stunner_gateway_operator_reconcile_events_total",
		Help: "Total number of reconcile events received by the operator event loop.",
	}, []string{"result"})

	// Generation is the current config generation number maintained by the operator.
	Generation = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "stunner_gateway_operator_generation",
		Help: "Current config generation number.",
	})

	// GenerationLastAcked is the generation number of the last update acknowledged by
	// the updater thread.
	GenerationLastAcked = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "stunner_gateway_operator_generation_last_acked",
		Help: "Generation number of the last update acknowledged by the updater thread.",
	})

	// OperatorLoopLastActive is the unix timestamp of the most recent operator
	// main-loop select iteration. A stale value proves the loop is wedged.
	OperatorLoopLastActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "stunner_gateway_operator_operator_loop_last_active_timestamp_seconds",
		Help: "Unix timestamp of the last operator main-loop select iteration; stale values indicate the loop is hung.",
	})

	// RendererLoopLastActive is the unix timestamp of the most recent renderer
	// goroutine select iteration.
	RendererLoopLastActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "stunner_gateway_operator_renderer_loop_last_active_timestamp_seconds",
		Help: "Unix timestamp of the last renderer goroutine select iteration; stale values indicate the loop is hung.",
	})

	// UpdaterLoopLastActive is the unix timestamp of the most recent updater
	// goroutine select iteration.
	UpdaterLoopLastActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "stunner_gateway_operator_updater_loop_last_active_timestamp_seconds",
		Help: "Unix timestamp of the last updater goroutine select iteration; stale values indicate the loop is hung.",
	})
)

// In-process mirrors of the heartbeat gauges. Stored as atomic unix-seconds so
// the /healthz check can read them without going through the prometheus client's
// dto.Metric write path. Zero means "never recorded yet" — treat as healthy
// until first heartbeat fires.
var (
	operatorLoopLastActive atomic.Int64
	rendererLoopLastActive atomic.Int64
	updaterLoopLastActive  atomic.Int64
)

// RecordOperatorHeartbeat marks the operator main loop as alive at the current
// time. Call this from every branch of the operator select loop.
func RecordOperatorHeartbeat() {
	now := time.Now().Unix()
	operatorLoopLastActive.Store(now)
	OperatorLoopLastActive.Set(float64(now))
}

// RecordRendererHeartbeat marks the renderer goroutine as alive at the current time.
func RecordRendererHeartbeat() {
	now := time.Now().Unix()
	rendererLoopLastActive.Store(now)
	RendererLoopLastActive.Set(float64(now))
}

// RecordUpdaterHeartbeat marks the updater goroutine as alive at the current time.
func RecordUpdaterHeartbeat() {
	now := time.Now().Unix()
	updaterLoopLastActive.Store(now)
	UpdaterLoopLastActive.Set(float64(now))
}

// OperatorLoopAge returns the time since the operator main loop last ran a
// select iteration. Returns 0 if no heartbeat has been recorded yet.
func OperatorLoopAge() time.Duration { return loopAge(&operatorLoopLastActive) }

// RendererLoopAge returns the time since the renderer goroutine last ran a
// select iteration. Returns 0 if no heartbeat has been recorded yet.
func RendererLoopAge() time.Duration { return loopAge(&rendererLoopLastActive) }

// UpdaterLoopAge returns the time since the updater goroutine last ran a
// select iteration. Returns 0 if no heartbeat has been recorded yet.
func UpdaterLoopAge() time.Duration { return loopAge(&updaterLoopLastActive) }

func loopAge(a *atomic.Int64) time.Duration {
	ts := a.Load()
	if ts == 0 {
		return 0
	}
	return time.Since(time.Unix(ts, 0))
}

func init() {
	ctrlmetrics.Registry.MustRegister(
		RenderTotal,
		RenderDuration,
		UpdateTotal,
		UpdateErrors,
		UpdateDuration,
		ResourceOperationsTotal,
		ReconcileEventsTotal,
		Generation,
		GenerationLastAcked,
		OperatorLoopLastActive,
		RendererLoopLastActive,
		UpdaterLoopLastActive,
	)
}
