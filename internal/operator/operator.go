package operator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	licensemgr "github.com/l7mp/stunner-gateway-operator/internal/licensemanager"
	"github.com/l7mp/stunner-gateway-operator/internal/metrics"
	"github.com/l7mp/stunner-gateway-operator/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
)

type controllerConstructor = func(manager.Manager, chan<- event.Event, logr.Logger) (controllers.Controller, error)

const (
	channelBufferSize = 200
	// finalizeTimeout bounds the finalizer's Kubernetes writes on shutdown.
	finalizeTimeout = 10 * time.Second
)

type OperatorConfig struct {
	Manager        manager.Manager
	ControllerName string
	LicenseManager licensemgr.Manager
	Renderer       renderer.Renderer
	Updater        *updater.Updater
	CDSServer      *config.Server
	Logger         logr.Logger
}

// Operator is the event dispatcher between the controllers, the renderer, the updater and the
// config discovery server. It registers the controllers with the manager and runs as a
// leader-only manager runnable: Start blocks until the context ends.
type Operator struct {
	mgr                           manager.Manager
	controllers                   []controllers.Controller
	operatorCh                    chan event.Event
	renderCh, updaterCh, configCh chan event.Event
	renderer                      renderer.Renderer
	updater                       *updater.Updater
	tracker                       *config.ProgressTracker
	progressReporters             []config.ProgressReporter
	finalizer                     bool
	gen, lastAckedGen             int
	ackLock                       sync.RWMutex
	log                           logr.Logger
}

// NewOperator creates the operator, registers the controllers with the manager and wires the
// subsystems to the operator channel.
func NewOperator(cfg OperatorConfig) (*Operator, error) {
	config.ControllerName = cfg.ControllerName

	o := &Operator{
		mgr:          cfg.Manager,
		operatorCh:   make(chan event.Event, channelBufferSize),
		renderCh:     cfg.Renderer.GetRenderChannel(),
		updaterCh:    cfg.Updater.GetUpdaterChannel(),
		configCh:     cfg.CDSServer.GetConfigUpdateChannel(),
		renderer:     cfg.Renderer,
		updater:      cfg.Updater,
		tracker:      config.NewProgressTracker(),
		finalizer:    config.EnableFinalizer,
		gen:          0,
		lastAckedGen: -1,
		log:          cfg.Logger.WithName("operator"),
	}
	o.progressReporters = []config.ProgressReporter{cfg.Renderer, cfg.Updater, cfg.CDSServer}

	cfg.LicenseManager.SetOperatorChannel(o.operatorCh)
	cfg.Renderer.SetOperatorChannel(o.operatorCh)
	cfg.Updater.SetAckChannel(o.operatorCh)

	for _, ctor := range []controllerConstructor{
		controllers.NewGatewayConfigController,
		controllers.NewDataplaneController,
		controllers.NewGatewayController,
		controllers.NewRouteController,
		controllers.NewNodeController,
	} {
		c, err := ctor(o.mgr, o.operatorCh, cfg.Logger)
		if err != nil {
			return nil, fmt.Errorf("Cannot register controller: %w", err)
		}
		o.controllers = append(o.controllers, c)
		o.log.V(3).Info("Registered controller", "name", c.Name())
	}

	return o, nil
}

// Start runs the event loop until the context ends, then the termination sequence.
func (o *Operator) Start(ctx context.Context) error {
	o.eventLoop(ctx)
	o.Terminate()
	return nil
}

func (o *Operator) eventLoop(ctx context.Context) {
	throttler := time.NewTicker(config.ThrottleTimeout)
	throttler.Stop()
	throttling := false

	// The first render waits until every controller reported one reconcile: a controller
	// reconciles only after its caches synced and each reconcile re-lists everything, so the
	// store is complete. The license status must be known too, or the first render would be
	// an unlicensed one. The gate timer is the circuit breaker for a controller that has
	// nothing to reconcile.
	pending := make(map[controllers.ControllerName]bool, len(o.controllers))
	for _, c := range o.controllers {
		pending[c.Name()] = true
	}
	licensePending := true
	gate := time.NewTimer(config.StartupRenderTimeout)
	defer gate.Stop()
	gated := func() bool { return len(pending) > 0 || licensePending }

	requestRender := func(e event.Event) {
		// rate-limit rendering requests before passing on to the renderer
		if throttling {
			metrics.ReconcileEventsTotal.WithLabelValues("throttled").Inc()
			o.log.V(3).Info("Rendering request throttled", "event", e.String())
			return
		}

		metrics.ReconcileEventsTotal.WithLabelValues("passed").Inc()
		throttling = true
		throttler.Reset(config.ThrottleTimeout)
		o.tracker.ProgressUpdate(1)

		o.log.V(3).Info("Initiating new rendering request", "event", e.String())
	}

	// Independent heartbeat ticker, since the throttler above is Stop()ed during
	// idle periods, so it cannot double as a liveness signal.
	heartbeat := time.NewTicker(metrics.LoopHeartbeatInterval)
	defer heartbeat.Stop()
	metrics.RecordOperatorHeartbeat()

	for {
		select {

		case <-heartbeat.C:
			metrics.RecordOperatorHeartbeat()

		case e := <-o.operatorCh:
			metrics.RecordOperatorHeartbeat()
			switch e.GetType() {
			case event.EventTypeUpdate:
				if n, _ := event.SendCoalesced(ctx, o.updaterCh, e); n > 0 {
					o.log.V(3).Info("Coalesced stale updater events", "count", n)
				}
				if n, _ := event.SendCoalesced(ctx, o.configCh, e); n > 0 {
					o.log.V(3).Info("Coalesced stale config-discovery events", "count", n)
				}

			case event.EventTypeReconcile:
				if len(pending) > 0 {
					delete(pending, controllers.ControllerName(e.(*event.EventReconcile).Sender))
					if len(pending) == 0 {
						o.log.Info("All controllers reported, startup gate open")
					}
				}
				if gated() {
					metrics.ReconcileEventsTotal.WithLabelValues("gated").Inc()
					o.log.V(3).Info("Rendering request held by the startup gate",
						"event", e.String(), "pending-controllers", len(pending),
						"license-pending", licensePending)
					continue
				}
				requestRender(e)

			case event.EventTypeLicense:
				// Every rendered update carries the license status too, so this
				// forward only matters while there is nothing to render, when no
				// update would carry it. Dropping it costs the config discovery
				// server a stale license report until the next render, which is
				// cheaper than blocking the loop on a busy config server.
				if !event.SendOrDrop(o.configCh, e) {
					o.log.V(3).Info("Dropping license event: config-discovery channel full")
				}
				licensePending = false
				if gated() {
					continue
				}
				requestRender(e)

			case event.EventTypeAck:
				gen := e.(*event.EventAck).Generation
				o.setLastAckedGeneration(gen)
				metrics.GenerationLastAcked.Set(float64(gen))

			default:
				o.log.Info("Internal error: operator received a request it should "+
					"never receive", "type", e.String(),
					"event-dump", fmt.Sprintf("%#v", e))
			}

		case <-gate.C:
			if !gated() {
				continue
			}
			names := []string{}
			for n := range pending {
				names = append(names, string(n))
			}
			o.log.Info("Startup gate timed out, rendering with what the controllers reported",
				"timeout", config.StartupRenderTimeout.String(), "pending-controllers", names,
				"license-pending", licensePending)
			pending = nil
			licensePending = false
			requestRender(event.NewEventReconcile("startup-gate"))

		case <-throttler.C:
			metrics.RecordOperatorHeartbeat()
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
			metrics.Generation.Set(float64(o.gen))
			event.Send(ctx, o.renderCh, event.NewEventRender(o.gen))

		case <-ctx.Done():
			return
		}
	}
}

// Terminate completes the termination sequence of the operator: it waits for the in-flight
// renders and updates to finish and runs the finalizer if enabled.
func (o *Operator) Terminate() {
	o.log.Info("Commencing termination sequence", "generation", o.gen)

	o.Stabilize()
	o.Stabilize()

	if o.finalizer {
		o.Finalize()
	}
}

// Finalize invalidates the status on all the managed resources and removes the managed
// dataplanes. It runs synchronously, after the event loop has stopped, with its own deadline:
// the manager context is already cancelled by then.
func (o *Operator) Finalize() {
	finalGen := o.gen + 1
	o.log.Info("Commencing finalizer sequence", "generation", finalGen,
		"last-acked-generation", o.GetLastAckedGeneration())

	if o.renderer == nil || o.updater == nil {
		return
	}

	u := o.renderer.Finalize(finalGen)
	if u == nil {
		o.log.V(2).Info("Nothing to finalize")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), finalizeTimeout)
	defer cancel()
	if err := o.updater.ProcessUpdate(ctx, u); err != nil {
		o.log.Error(err, "Could not apply the finalizer update", "update", u.String())
		return
	}
	o.setLastAckedGeneration(finalGen)
}
