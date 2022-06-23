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
const clusterTimeout = 10 * time.Second

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
		operatorCh: make(chan event.Event, 5),
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

	go o.eventLoop(ctx)

	return nil
}

func (o *Operator) eventLoop(ctx context.Context) {
	defer close(o.operatorCh)

	for {
		select {
		case e := <-o.operatorCh:
			if err := o.ProcessEvent(e); err != nil {
				o.log.Error(err, "could not process controller event",
					"event", e.String())
			}

		case <-ctx.Done():
			return
		}
	}
}

// ProcessEvent dispatches an event to the corresponding renderer
func (o *Operator) ProcessEvent(e event.Event) error {
	switch e.GetType() {
	case event.EventTypeUpdate:
		// pass through to the updater
		o.updaterCh <- e

	case event.EventTypeUpsert:
		// reflect!
		e := e.(*event.EventUpsert)

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
		default:
			return fmt.Errorf("could not process event %q for an unknown object of type %q",
				e.String(), store.GetObjectKey(e.Object))
		}

		// trigger the render event
		o.renderCh <- event.NewEventRender(store.GetObjectKey(e.Object), "upsert")

	case event.EventTypeDelete:
		// reflect!
		e := e.(*event.EventDelete)

		key := types.NamespacedName{
			Namespace: e.Object.GetNamespace(),
			Name:      e.Object.GetName(),
		}

		switch e.Object.(type) {
		case *gatewayv1alpha2.GatewayClass:
			store.GatewayClasses.Remove(key)
		case *stunnerv1alpha1.GatewayConfig:
			store.GatewayConfigs.Remove(key)
		case *gatewayv1alpha2.Gateway:
			store.Gateways.Remove(key)
		case *gatewayv1alpha2.UDPRoute:
			store.UDPRoutes.Remove(key)
		case *corev1.Service:
			store.Services.Remove(key)
		default:
			return fmt.Errorf("could not process event %q for an unknown object of type %q",
				e.String(), key.String())
		}

		// trigger the render event
		o.renderCh <- event.NewEventRender(store.GetObjectKey(e.Object), "delete")

	}

	return nil
}
