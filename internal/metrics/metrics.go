package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

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
)

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
	)
}
