package metrics

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoopAgeZeroBeforeFirstHeartbeat(t *testing.T) {
	var a atomic.Int64
	assert.Equal(t, time.Duration(0), loopAge(&a), "loopAge with no heartbeat")
}

func TestLoopAgeAfterHeartbeat(t *testing.T) {
	var a atomic.Int64
	a.Store(time.Now().Add(-30 * time.Second).Unix())

	got := loopAge(&a)
	assert.GreaterOrEqual(t, got, 25*time.Second, "loopAge lower bound")
	assert.LessOrEqual(t, got, 35*time.Second, "loopAge upper bound")
}

func TestRecordHeartbeats(t *testing.T) {
	cases := []struct {
		name   string
		record func()
		age    func() time.Duration
		mirror *atomic.Int64
	}{
		{"operator", RecordOperatorHeartbeat, OperatorLoopAge, &operatorLoopLastActive},
		{"renderer", RecordRendererHeartbeat, RendererLoopAge, &rendererLoopLastActive},
		{"updater", RecordUpdaterHeartbeat, UpdaterLoopAge, &updaterLoopLastActive},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mirror.Store(0)
			assert.Equal(t, time.Duration(0), tc.age(), "age before first heartbeat")

			before := time.Now().Unix()
			tc.record()
			after := time.Now().Unix()

			stored := tc.mirror.Load()
			assert.GreaterOrEqual(t, stored, before, "mirror lower bound")
			assert.LessOrEqual(t, stored, after, "mirror upper bound")
			assert.LessOrEqual(t, tc.age(), 2*time.Second, "age right after heartbeat")
		})
	}
}

func TestStalenessThresholdAboveHistogramCap(t *testing.T) {
	// The reconcile-time histograms top out at 60s; the staleness threshold
	// must sit comfortably above that so slow-but-alive iterations aren't
	// flagged as hung.
	assert.Greater(t, LoopStalenessThreshold, 60*time.Second, "staleness > histogram cap")
	assert.Less(t, LoopHeartbeatInterval, LoopStalenessThreshold, "heartbeat < staleness")
}
