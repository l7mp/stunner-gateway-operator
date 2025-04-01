package operator

import (
	// "fmt"

	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// GetManager returns the controller manager associated with this operator
func (o *Operator) GetManager() manager.Manager {
	return o.manager
}

// GetLogger returns the logger associated with this operator
func (o *Operator) GetLogger() logr.Logger {
	return o.logger
}

// GetOperatorChannel returns the channel on which the operator event dispatcher listens
func (o *Operator) GetOperatorChannel() event.EventChannel {
	return o.operatorCh
}

// // GetControllerName returns the controller-name (as per GatewayClass.Spec.ControllerName)
// // associated with this operator
// func (o *Operator) GetControllerName() string {
// 	return o.controllerName
// }

// SetProgressReporters sets the operator subsystems that need to be queried to check the number of
// operations in progrses. This can be used to implement graceful shutdown.
func (o *Operator) SetProgressReporters(reporters ...config.ProgressReporter) {
	o.progressReporters = make([]config.ProgressReporter, len(reporters))
	copy(o.progressReporters, reporters)
}

// ProgressReport returns the number of ongoing operations (rendering processes, updates, etc) plus
// the number of throttled rendering processes in progress.
func (o *Operator) ProgressReport() int {
	progress := 0
	for _, r := range o.progressReporters {
		progress += r.ProgressReport()
	}

	op := o.tracker.ProgressReport()
	return progress + op
}

// GetLastAckedGeneration returns the last update generation acknowledged by the updater.
func (o *Operator) GetLastAckedGeneration() int {
	o.ackLock.RLock()
	defer o.ackLock.RUnlock()
	return o.lastAckedGen
}

// setLastAckedGeneration sets the last update generation acknowledged by the updater.
func (o *Operator) setLastAckedGeneration(gen int) {
	o.ackLock.Lock()
	defer o.ackLock.Unlock()
	o.lastAckedGen = gen
}

// Stabilize waits until all internal progress has stopped by checking if there's no activity 3 times.
func (o *Operator) Stabilize() {
	d := 50 * time.Millisecond
	start := time.Now()
	stabilizer := func() {
		for {
			progress := o.ProgressReport()
			// o.log.V(2).Info("total progress report", "report", progress)
			if progress != 0 {
				time.Sleep(d)
			} else {
				return
			}
		}
	}

	stabilizer()
	time.Sleep(d)
	stabilizer()
	time.Sleep(d)
	stabilizer()

	o.log.Info("Operator has stabilized: progress counter reports no ongoing operations in 3 consecutive queries",
		"duration", time.Since(start), "timeout-between-queries", d)
}

// SetFinalizer can be used to prevent the finalizer from running on termination. This is useful for testing.
func (o *Operator) SetFinalizer(state bool) {
	o.finalizer = state
}
