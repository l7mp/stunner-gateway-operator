package operator

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/core/v1"
	// corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
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
	ControllerName string
	Manager        manager.Manager
	RenderCh       chan event.Event
	UpdaterCh      chan event.Event
	Logger         logr.Logger
}

type Operator struct {
	ctx                             context.Context
	mgr                             manager.Manager
	controllerName                  string
	gatewayClassStore               store.Store
	gatewayConfigStore              store.Store
	gatewayStore                    store.Store
	udpRouteStore                   store.Store
	serviceStore                    store.Store
	renderCh, operatorCh, updaterCh chan event.Event
	manager                         manager.Manager
	log, logger                     logr.Logger
}

// NewOperator creates a new Operator
func NewOperator(cfg OperatorConfig) *Operator {
	return &Operator{
		controllerName: cfg.ControllerName,
		mgr:            cfg.Manager,
		renderCh:       cfg.RenderCh,
		operatorCh:     make(chan event.Event, 5),
		updaterCh:      cfg.UpdaterCh,
		logger:         cfg.Logger,
	}
}

func (o *Operator) Start(ctx context.Context) error {
	log := o.logger.WithName("operator")
	o.log = log
	o.ctx = ctx

	if o.mgr == nil {
		return fmt.Errorf("controller runtime manager uninitialized")
	}

	o.SetupStore()

	log.V(3).Info("starting GatewayClass controller")
	err := stunnerctrl.RegisterGatewayClassController(o.mgr, o.gatewayClassStore, o.controllerName)
	if err != nil {
		return fmt.Errorf("cannot register gateway-class controller: %w", err)
	}

	log.V(3).Info("starting GatewayConfig controller")
	err = stunnerctrl.RegisterGatewayConfigController(o.mgr, o.gatewayConfigStore, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register gatewayconfig controller: %w", err)
	}

	log.V(3).Info("starting Gateway controller")
	err = stunnerctrl.RegisterGatewayController(o.mgr, o.gatewayStore, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register gateway controller: %w", err)
	}

	log.V(3).Info("starting UDPRoute controller")
	err = stunnerctrl.RegisterUDPRouteController(o.mgr, o.udpRouteStore, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register udproute controller: %w", err)
	}

	log.V(3).Info("starting Service controller")
	err = stunnerctrl.RegisterServiceController(o.mgr, o.serviceStore, o.operatorCh)
	if err != nil {
		return fmt.Errorf("cannot register service controller: %w", err)
	}

	go o.eventLoop(ctx)

	return nil
}

func (o *Operator) SetupStore() {
	o.gatewayClassStore = store.NewStore()  //o.logger.WithName("gateway-class-store"))
	o.gatewayConfigStore = store.NewStore() //o.logger.WithName("gatewayconfig-store"))
	o.gatewayStore = store.NewStore()       //o.logger.WithName("gateway-store"))
	o.udpRouteStore = store.NewStore()      //o.logger.WithName("udproute-store"))
	o.serviceStore = store.NewStore()       //o.logger.WithName("service-store"))
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

func (o *Operator) GetOperatorChannel() chan event.Event {
	return o.operatorCh
}

// ProcessEvent dispatches an event to the corresponding renderer
func (o *Operator) ProcessEvent(e event.Event) error {
	switch e.GetType() {
	case event.EventTypeRender:
		// pass through to the renderer
		o.renderCh <- e
	case event.EventTypeUpdate:
		// pass through to the updater
		o.updaterCh <- e
	}

	return nil
}
