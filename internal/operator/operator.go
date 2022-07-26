package operator

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	// ctlr "sigs.k8s.io/controller-runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// clusterTimeout is a timeout for connections to the Kubernetes API
const (
	channelBufferSize = 200
	throttleTimeout   = 250 * time.Millisecond
)

var scheme = runtime.NewScheme()

func init() {
	_ = gatewayv1alpha2.AddToScheme(scheme)
	_ = stunnerv1alpha1.AddToScheme(scheme)
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
	err := stunnerctrl.RegisterGatewayClassController(o.mgr, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register gateway-class controller: %w", err)
	}

	log.V(3).Info("starting GatewayConfig controller")
	err = stunnerctrl.RegisterGatewayConfigController(o.mgr, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register gatewayconfig controller: %w", err)
	}

	log.V(3).Info("starting Gateway controller")
	err = stunnerctrl.RegisterGatewayController(o.mgr, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register gateway controller: %w", err)
	}

	log.V(3).Info("starting UDPRoute controller")
	err = stunnerctrl.RegisterUDPRouteController(o.mgr, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register udproute controller: %w", err)
	}

	log.V(3).Info("starting Service controller")
	err = stunnerctrl.RegisterServiceController(o.mgr, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register service controller: %w", err)
	}

	log.V(3).Info("starting Node controller")
	err = stunnerctrl.RegisterNodeController(o.mgr, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register node controller: %w", err)
	}

	if config.EnableEndpointDiscovery {
		log.V(3).Info("starting Endpoint controller")
		err = stunnerctrl.RegisterEndpointController(o.mgr, o.operatorCh)
		if err != nil {
			return fmt.Errorf("cannot register endpoint controller: %w", err)
		}
	}

	go o.eventLoop(ctx)

	return nil
}

func (o *Operator) eventLoop(ctx context.Context) {
	defer close(o.operatorCh)

	throttler := time.NewTicker(throttleTimeout)
	throttler.Stop()
	throttling := false

	for {
		select {

		case e := <-o.operatorCh:
			switch e.GetType() {
			case event.EventTypeUpdate:
				// pass through to the updater
				o.updaterCh <- e

			case event.EventTypeUpsert:
				e := e.(*event.EventUpsert)
				if err := o.ProcessUpsertEvent(e); err != nil {
					o.log.Error(err, "could not process upsert event",
						"event", e.String())
					continue
				}

				if !config.EnableRenderThrottling {
					// fire immediately
					o.renderCh <- event.NewEventRender()
					continue
				}

				// render request in progress: do nothing
				if throttling {
					o.log.V(4).Info("rendering request throttled", "event",
						e.String())
					continue
				}

				// request a new rendering round
				throttling = true
				throttler.Reset(throttleTimeout)

				o.log.V(4).Info("initiating new rendering request", "event",
					e.String())

			case event.EventTypeDelete:
				e := e.(*event.EventDelete)
				if err := o.ProcessDeleteEvent(e); err != nil {
					o.log.Error(err, "could not process delete event",
						"event", e.String())
					continue
				}

				if !config.EnableRenderThrottling {
					// fire immediately
					o.renderCh <- event.NewEventRender()
					continue
				}

				// render request in progress: do nothing
				if throttling {
					o.log.V(4).Info("rendering request throttled", "event",
						e.String())
					continue
				}

				// request a new rendering round
				throttling = true
				throttler.Reset(throttleTimeout)

				o.log.V(4).Info("initiating new rendering request", "event",
					e.String())

			default:
				o.log.Info("internal error: unknown event %#v", e)
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

// ProcessUpsertEvent dispatches an event to the corresponding renderer
func (o *Operator) ProcessUpsertEvent(e *event.EventUpsert) error {
	key := types.NamespacedName{
		Namespace: e.Object.GetNamespace(),
		Name:      e.Object.GetName(),
	}

	o.log.V(1).Info("processing upsert event", "event", e.String(), "object", key.String())

	switch e.Object.(type) {
	case *gatewayv1alpha2.GatewayClass:
		store.GatewayClasses.Upsert(e.Object)
	case *stunnerv1alpha1.GatewayConfig:
		store.GatewayConfigs.Upsert(e.Object)
	case *gatewayv1alpha2.Gateway:
		store.Gateways.Upsert(e.Object)
	case *gatewayv1alpha2.UDPRoute:
		store.UDPRoutes.Upsert(e.Object)
	case *corev1.Service:
		store.Services.Upsert(e.Object)
	case *corev1.Node:
		store.Nodes.Upsert(e.Object)
	case *corev1.Endpoints:
		store.Endpoints.Upsert(e.Object)
	default:
		return fmt.Errorf("could not process event %q for an unknown object %q",
			e.String(), key.String())
	}

	return nil
}

// ProcessDeleteEvent dispatches an event to the corresponding renderer
func (o *Operator) ProcessDeleteEvent(e *event.EventDelete) error {
	o.log.V(1).Info("processing delete event", "event", e.String())

	switch e.Kind {
	case event.EventKindGatewayClass:
		store.GatewayClasses.Remove(e.Key)
	case event.EventKindGatewayConfig:
		store.GatewayConfigs.Remove(e.Key)
	case event.EventKindGateway:
		store.Gateways.Remove(e.Key)
	case event.EventKindUDPRoute:
		store.UDPRoutes.Remove(e.Key)
	case event.EventKindService:
		store.Services.Remove(e.Key)
	case event.EventKindNode:
		store.Nodes.Remove(e.Key)
	case event.EventKindEndpoint:
		store.Endpoints.Remove(e.Key)
	default:
		return fmt.Errorf("could not process event %q for an unknown object %q",
			e.String(), e.Key.String())
	}

	return nil
}
