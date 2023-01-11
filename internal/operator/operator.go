package operator

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// clusterTimeout is a timeout for connections to the Kubernetes API
const (
	channelBufferSize = 200
)

var scheme = runtime.NewScheme()

func init() {
	_ = gwapiv1a2.AddToScheme(scheme)
	_ = stnrv1a1.AddToScheme(scheme)
	_ = apiv1.AddToScheme(scheme)
}

type OperatorConfig struct {
	Manager        manager.Manager
	ControllerName string
	RenderCh       chan event.Event
	UpdaterCh      chan event.Event
	Logger         logr.Logger
}

type Operator struct {
	ctx                             context.Context
	mgr                             manager.Manager
	renderCh, operatorCh, updaterCh chan event.Event
	manager                         manager.Manager
	log, logger                     logr.Logger
}

// NewOperator creates a new Operator
func NewOperator(cfg OperatorConfig) *Operator {
	config.ControllerName = cfg.ControllerName

	return &Operator{
		mgr:        cfg.Manager,
		renderCh:   cfg.RenderCh,
		operatorCh: make(chan event.Event, channelBufferSize),
		updaterCh:  cfg.UpdaterCh,
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

	log.V(3).Info("starting GatewayClass controller", "controller-name",
		config.ControllerName)
	if err := controllers.RegisterGatewayClassController(o.mgr, o.operatorCh, o.logger); err != nil {
		return fmt.Errorf("cannot register gateway-class controller: %w", err)
	}

	log.V(3).Info("starting GatewayConfig controller")
	if err := controllers.RegisterGatewayConfigController(o.mgr, o.operatorCh, o.logger); err != nil {
		return fmt.Errorf("cannot register gatewayconfig controller: %w", err)
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

				o.log.V(3).Info("initiating new rendering request", "event",
					e.String())

			default:
				o.log.Info("internal error: unknown event received %#v", e)
				continue
			}

		case <-throttler.C:
			o.renderCh <- event.NewEventRender()
			throttling = false
			throttler.Stop()

		case <-ctx.Done():
			// FIXME revert gateway-class status to "Waiting..."
			return
		}
	}
}
