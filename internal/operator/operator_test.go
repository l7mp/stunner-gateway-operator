package operator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
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

// TestEventLoopDoesNotBlockOnSlowCDSConsumer is a regression test for the deadlock where a
// slow or absent CDS (config-discovery) consumer caused the operator event loop to hang
// permanently, stopping all udproute/gateway/dataplane reconciliation.
//
// Root cause: the event loop did a blocking send `o.configCh <- e`. The CDS server calls
// UpdateConfig() which fans out configs over gRPC to every connected stunnerd pod. With many
// (or slow) pods this takes longer than ThrottleTimeout, so configCh (buffer=10) fills up
// and the blocking send deadlocks the single event-loop goroutine. All subsequent reconcile
// events then pile up unprocessed; only "node-controller Reconciling" log lines appear.
//
// Fix: replace the blocking send with a drain-then-non-blocking-send pattern.
//
// To observe the regression intentionally, revert the configCh dispatch in eventLoop to:
//
//	o.configCh <- e
//
// The test will then fail at event 11 (configCh buffer=10) with a deadlock message.
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
			// event i reached the updater — loop is alive and processing
		case <-deadline:
			t.Fatalf("deadlock: only %d/%d updates reached updaterCh within 3s "+
				"(configCh cap=%d, old blocking send deadlocks at event %d). "+
				"The CDS channel blocking bug has regressed.",
				i, numUpdates, cap(configCh), cap(configCh)+1)
		}
	}

	// configCh should hold at most its buffer capacity — the non-blocking drain+send pattern
	// means typically only the latest snapshot is queued at any given moment.
	if got := len(configCh); got > cap(configCh) {
		t.Errorf("configCh has %d items, expected ≤ cap (%d)", got, cap(configCh))
	}

	// updaterCh should be drained by now.
	if got := len(updaterCh); got != 0 {
		t.Errorf("updaterCh has %d unexpected leftover items", got)
	}

	t.Logf("OK: all %d updates processed with a permanently-blocked configCh consumer "+
		"(configCh has %d item(s) queued — the latest snapshot)",
		numUpdates, len(configCh))
}

// TestEventLoopStillProcessesReconcileWhileCDSBlocked verifies that EventTypeReconcile events
// continue to be processed — and the throttler fires — even when configCh is saturated.
//
// This is the observable symptom of the original bug: only "node-controller Reconciling"
// log lines appeared because the controllers could still enqueue EventTypeReconcile events
// into the 200-slot operatorCh buffer, but the deadlocked loop consumed none of them.
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
		t.Fatal("deadlock: EventTypeReconcile not processed within 3s with a " +
			fmt.Sprintf("saturated configCh (cap=%d). ", cap(configCh)) +
			"The event loop is blocked by a blocking configCh send.")
	}
}
