package operator

import (
	"context"
	"fmt"
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
	renderCh, operatorCh, updaterCh, configCh chan event.Event
	manager                                   manager.Manager
	tracker                                   *config.ProgressTracker
	progressReporters                         []config.ProgressReporter
	log, logger                               logr.Logger
}

// NewOperator creates a new Operator
func NewOperator(cfg OperatorConfig) *Operator {
	config.ControllerName = cfg.ControllerName

	return &Operator{
		mgr:        cfg.Manager,
		renderCh:   cfg.RenderCh,
		operatorCh: make(chan event.Event, channelBufferSize),
		updaterCh:  cfg.UpdaterCh,
		configCh:   cfg.ConfigCh,
		tracker:    config.NewProgressTracker(),
		logger:     cfg.Logger,
	}
}

func (o *Operator) Start(ctx context.Context) error {
	log := o.logger.WithName("operator")
	o.log = log
	o.ctx = ctx

	if o.mgr == nil {
		return fmt.Errorf("controller runtime manager uninitialized")
	}

	log.V(3).Info("starting GatewayConfig controller")
	if err := controllers.RegisterGatewayConfigController(o.mgr, o.operatorCh, o.logger); err != nil {
		return fmt.Errorf("cannot register gatewayconfig controller: %w", err)
	}

	log.V(3).Info("starting Dataplane controller")
	if err := controllers.RegisterDataplaneController(o.mgr, o.operatorCh, o.logger); err != nil {
		return fmt.Errorf("cannot register dataplane controller: %w", err)
	}

	log.V(3).Info("starting Gateway controller")
	if err := controllers.RegisterGatewayController(o.mgr, o.operatorCh, o.logger); err != nil {
		return fmt.Errorf("cannot register gateway controller: %w", err)
	}

	log.V(3).Info("starting UDPRoute controller")
	if err := controllers.RegisterUDPRouteController(o.mgr, o.operatorCh, o.logger); err != nil {
		return fmt.Errorf("cannot register udproute controller: %w", err)
	}

	log.V(3).Info("starting Node controller")
	if err := controllers.RegisterNodeController(o.mgr, o.operatorCh, o.logger); err != nil {
		return fmt.Errorf("cannot register node controller: %w", err)
	}

	go o.eventLoop(ctx)

	return nil
}

func (o *Operator) eventLoop(ctx context.Context) {
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

			case event.EventTypeRender:
				// rate-limit rendering requests before passing on to the renderer
				// render request in progress: do nothing
				if throttling {
					o.log.V(3).Info("rendering request throttled", "event",
						e.String())
					continue
				}

				// request a new rendering round
				throttling = true
				throttler.Reset(config.ThrottleTimeout)
				o.tracker.ProgressUpdate(1)

				o.log.V(3).Info("initiating new rendering request", "event",
					e.String())

			default:
				o.log.Info("internal error: unknown event received %#v", e)
				continue
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
			o.renderCh <- event.NewEventRender()

		case <-ctx.Done():
			// FIXME revert gateway-class status to "Waiting..."
			return
		}
	}
}

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
