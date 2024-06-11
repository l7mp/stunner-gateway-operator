package operator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/event"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

// clusterTimeout is a timeout for connections to the Kubernetes API
const (
	channelBufferSize = 200
)

var scheme = runtime.NewScheme()

func init() {
	_ = gwapiv1a2.AddToScheme(scheme)
	_ = gwapiv1.AddToScheme(scheme)
	_ = stnrgwv1.AddToScheme(scheme)
	_ = apiv1.AddToScheme(scheme)
}

type OperatorConfig struct {
	Manager        manager.Manager
	ControllerName string
	RenderCh       chan event.Event
	ConfigCh       chan event.Event
	UpdaterCh      chan event.Event
	Logger         logr.Logger
}

type Operator struct {
	ctx                                       context.Context
	mgr                                       manager.Manager
	gwConfC, dpC, gwC, rouC, nodeC            controllers.Controller
	renderCh, operatorCh, updaterCh, configCh chan event.Event
	manager                                   manager.Manager
	tracker                                   *config.ProgressTracker
	progressReporters                         []config.ProgressReporter
	finalizer                                 bool
	gen, lastAckedGen                         int
	ackLock                                   sync.RWMutex
	log, logger                               logr.Logger
}

// NewOperator creates a new Operator
func NewOperator(cfg OperatorConfig) *Operator {
	config.ControllerName = cfg.ControllerName

	return &Operator{
		mgr:          cfg.Manager,
		renderCh:     cfg.RenderCh,
		operatorCh:   make(chan event.Event, channelBufferSize),
		updaterCh:    cfg.UpdaterCh,
		configCh:     cfg.ConfigCh,
		tracker:      config.NewProgressTracker(),
		finalizer:    true,
		gen:          0,
		lastAckedGen: -1,
		logger:       cfg.Logger,
	}
}

// Start spawns the Kubernetes controllers, enters the operator main loop and terminates when the
// provided context is canceled. On termination, Start calls the provided cancel function (if not
// nil) to signal that it has finished running. Pass in the manager context as the second argument
// to let the operator automatically cancel the manager on termination.
func (o *Operator) Start(ctx context.Context, cancel context.CancelFunc) error {
	log := o.logger.WithName("operator")
	o.log = log
	o.ctx = ctx

	if o.mgr == nil {
		return fmt.Errorf("Controller runtime manager uninitialized")
	}

	log.V(3).Info("Starting GatewayConfig controller")
	c, err := controllers.NewGatewayConfigController(o.mgr, o.operatorCh, o.logger)
	if err != nil {
		return fmt.Errorf("Cannot register gatewayconfig controller: %w", err)
	}
	o.gwConfC = c

	log.V(3).Info("Starting Dataplane controller")
	c, err = controllers.NewDataplaneController(o.mgr, o.operatorCh, o.logger)
	if err != nil {
		return fmt.Errorf("Cannot register dataplane controller: %w", err)
	}
	o.dpC = c

	log.V(3).Info("Starting Gateway controller")
	c, err = controllers.NewGatewayController(o.mgr, o.operatorCh, o.logger)
	if err != nil {
		return fmt.Errorf("Cannot register gateway controller: %w", err)
	}
	o.gwC = c

	log.V(3).Info("Starting UDPRoute controller")
	c, err = controllers.NewUDPRouteController(o.mgr, o.operatorCh, o.logger)
	if err != nil {
		return fmt.Errorf("Cannot register udproute controller: %w", err)
	}
	o.rouC = c

	log.V(3).Info("Starting Node controller")
	c, err = controllers.NewNodeController(o.mgr, o.operatorCh, o.logger)
	if err != nil {
		return fmt.Errorf("Cannot register node controller: %w", err)
	}
	o.nodeC = c

	go o.eventLoop(ctx, cancel)

	return nil
}

func (o *Operator) eventLoop(ctx context.Context, cancel context.CancelFunc) {
	defer close(o.operatorCh)

	throttler := time.NewTicker(config.ThrottleTimeout)
	throttler.Stop()
	throttling := false

	for {
		select {

		case e := <-o.operatorCh:
			switch e.GetType() {
			case event.EventTypeUpdate:
				// pass through to the updater
				o.updaterCh <- e

				// notify the config discovery server
				o.configCh <- e

			case event.EventTypeReconcile:
				// rate-limit rendering requests before passing on to the renderer
				// render request in progress: do nothing
				if throttling {
					o.log.V(3).Info("Rendering request throttled", "event",
						e.String())
					continue
				}

				// request a new rendering round
				throttling = true
				throttler.Reset(config.ThrottleTimeout)
				o.tracker.ProgressUpdate(1)

				o.log.V(3).Info("Initiating new rendering request", "event",
					e.String())

			case event.EventTypeAck:
				// administer
				o.setLastAckedGeneration(e.(*event.EventAck).Generation)

			default:
				o.log.Info("Internal error: operator received a request it should "+
					"never receive", "type", e.String(),
					"event-dump", fmt.Sprintf("%#v", e))
			}

		case <-throttler.C:
			throttling = false
			throttler.Stop()

			// if rendering takes more than the throttle timeout (1ms in the tests)
			// then the ticker may produce 2 ticks (the channel capacity is 1) and this
			// may result more than one rendering event
			if o.tracker.ProgressReport() > 0 {
				o.tracker.ProgressUpdate(-1)
			}

			o.log.Info("Starting new reconcile generation", "generation", o.gen,
				"last-acked-generation", o.GetLastAckedGeneration())
			o.gen += 1
			o.renderCh <- event.NewEventRender(o.gen)

		case <-ctx.Done():
			o.Terminate()

			if cancel != nil {
				cancel()
			}

			return
		}
	}
}

// Terminate completes the termination sequence of the operator.
func (o *Operator) Terminate() {
	o.log.Info("Commencing termination sequence", "generation", o.gen)

	// stop controllers (actually only prevent them from sending further reconcile events)
	o.gwConfC.Terminate()
	o.dpC.Terminate()
	o.gwC.Terminate()
	o.rouC.Terminate()
	o.nodeC.Terminate()

	// wait for ongoing activity to finish
	o.Stabilize()
	o.Stabilize()

	// perform the finalize sequence if requested
	if o.finalizer {
		o.Finalize()
	}
}

// Finalize invalidates the status on all the managed resources. Note that Finalize must be called
// with the main even loop blocked.
func (o *Operator) Finalize() {
	// get the last update generation
	lastGen := o.GetLastAckedGeneration()
	o.log.Info("Commencing finalizer sequence", "generation", o.gen, "last-acked-generation",
		lastGen)

	// send the finalize event to the renderer
	finalGen := o.gen + 1
	o.renderCh <- event.NewEventFinalize(finalGen)

	o.log.V(2).Info("Finalizer request sent to renderer, waiting for response",
		"last-acked-generation", lastGen)

	// event loop is blocked: we must handle message passing ourselves
	u := <-o.operatorCh

	o.log.V(2).Info("Renderer ready, initiating the updater", "event", u.String())

	// send to the updater
	o.updaterCh <- u

	// wait for the updater to finish with lastGen+1
	if o.GetLastAckedGeneration() == finalGen {
		return
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-o.operatorCh:
			if o.GetLastAckedGeneration() != finalGen {
				o.log.V(2).Info("Update ready, exiting finalizer",
					"gen", o.gen, "last-acked-generation", lastGen)
				return
			}

			o.log.V(2).Info("Ignoring out-of-order ack from updater",
				"gen", o.gen, "last-acked-generation", lastGen)

		case <-timeout:
			o.log.V(2).Info("Cound not finish the finalization sequence in 2 sec, exiting anyway")
		}
	}
}
