package operator

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// noopController satisfies controllers.Controller with no-op methods, allowing
// the operator to be instantiated without a real controller-runtime manager.
type noopController struct{}

func (c *noopController) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (c *noopController) Terminate() {}

// newTestOperator builds a minimal operator wired to the provided stub channels.
// No manager, no real controllers — only the fields used by eventLoop.
func newTestOperator(opCh chan event.Event, updaterCh, configCh, renderCh chan event.Event) *Operator {
	noop := &noopController{}
	return &Operator{
		operatorCh: event.NewEventChannel(opCh),
		configCh:   configCh,
		updaterCh:  updaterCh,
		renderCh:   renderCh,
		tracker:    config.NewProgressTracker(),
		finalizer:  false,
		gwConfC:    noop,
		dpC:        noop,
		gwC:        noop,
		rouC:       noop,
		nodeC:      noop,
		log:        logr.Discard(),
		logger:     logr.Discard(),
	}
}

// TestEventLoopDoesNotBlockOnSlowCDSConsumer asserts that the event loop continues
// delivering updates to the updater even when the CDS consumer is permanently absent.
func TestEventLoopDoesNotBlockOnSlowCDSConsumer(t *testing.T) {
	origThrottle := config.ThrottleTimeout
	config.ThrottleTimeout = 10 * time.Millisecond
	defer func() { config.ThrottleTimeout = origThrottle }()

	// numUpdates must exceed configCh's buffer size so the old blocking send deadlocks.
	// configCh buffer is 10; 30 updates guarantees the bug manifests.
	const numUpdates = 30

	// configCh is intentionally NEVER drained, simulating permanently slow/absent stunnerd
	// clients whose gRPC UpdateConfig() call takes longer than ThrottleTimeout.
	configCh := make(chan event.Event, 10)

	// updaterCh must be large enough not to back-pressure the loop under test.
	updaterCh := make(chan event.Event, numUpdates+5)

	// renderCh absorbs throttle-tick render events so it is not a secondary blocker.
	renderCh := make(chan event.Event, numUpdates+5)

	opCh := make(chan event.Event, channelBufferSize)
	o := newTestOperator(opCh, updaterCh, configCh, renderCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.operatorCh.Get()
	go o.eventLoop(ctx, nil)

	// Enqueue all updates into opCh (large buffer, so this is instant).
	for i := 0; i < numUpdates; i++ {
		opCh <- event.NewEventUpdate(i)
	}

	// The real assertion: wait for ALL updates to arrive on updaterCh.
	// This only succeeds if the event loop processed every event from opCh.
	//
	// Old code behaviour: the loop deadlocks after delivering event 10 to updaterCh
	// (the 11th blocking send to configCh never returns), so this loop stalls at i=11.
	//
	// Fixed behaviour: the loop never blocks on configCh and delivers all 30 events.
	deadline := time.After(3 * time.Second)
	for i := 0; i < numUpdates; i++ {
		select {
		case <-updaterCh:
			// event reached the updater — loop is alive
		case <-deadline:
			require.Failf(t, "deadlock",
				"only %d/%d updates reached updaterCh within 3s (configCh cap=%d)",
				i, numUpdates, cap(configCh))
		}
	}

	assert.LessOrEqual(t, len(configCh), cap(configCh), "configCh should not exceed its capacity")
	assert.Empty(t, updaterCh, "updaterCh should be fully drained")
}

// TestEventLoopStillProcessesReconcileWhileCDSBlocked asserts that reconcile events are
// processed and the throttler fires even when configCh is saturated.
func TestEventLoopStillProcessesReconcileWhileCDSBlocked(t *testing.T) {
	origThrottle := config.ThrottleTimeout
	config.ThrottleTimeout = 10 * time.Millisecond
	defer func() { config.ThrottleTimeout = origThrottle }()

	configCh := make(chan event.Event, 10) // never drained
	updaterCh := make(chan event.Event, 50)
	renderCh := make(chan event.Event, 50)

	opCh := make(chan event.Event, channelBufferSize)
	o := newTestOperator(opCh, updaterCh, configCh, renderCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.operatorCh.Get()
	go o.eventLoop(ctx, nil)

	// Saturate configCh with updates so the old code would deadlock.
	const numUpdates = 20
	for i := 0; i < numUpdates; i++ {
		opCh <- event.NewEventUpdate(i)
	}

	// Drain updaterCh in the background so it never becomes a secondary blocker.
	go func() {
		for {
			select {
			case <-updaterCh:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Send a reconcile event. It arms the throttle ticker which, after ThrottleTimeout
	// (10ms), sends an EventTypeRender to renderCh. Receiving that event proves the loop
	// is alive — it was not deadlocked by the saturated configCh.
	opCh <- event.NewEventReconcile()

	select {
	case <-renderCh:
		// throttle timer fired; loop is alive and responsive
	case <-time.After(3 * time.Second):
		require.Failf(t, "deadlock",
			"EventTypeReconcile not processed within 3s with saturated configCh (cap=%d)",
			cap(configCh))
	}
}
