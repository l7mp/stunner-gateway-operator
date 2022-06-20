package operator

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apiv1 "k8s.io/api/core/v1"
	// corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctlr "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
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
	RenderCh       chan event.Event
	Logger         logr.Logger
}

type Operator struct {
	ctx                               context.Context
	controllerName                    string
	gatewayClassStore                 store.Store
	gatewayConfigStore                store.Store
	gatewayStore                      store.Store
	udpRouteStore                     store.Store
	serviceStore                      store.Store
	renderCh, controllerCh, updaterCh chan event.Event
	manager                           manager.Manager
	updater                           *updater.Updater
	log, logger                       logr.Logger
}

// NewOperator creates a new Operator
func NewOperator(cfg OperatorConfig) *Operator {
	return &Operator{
		controllerName: cfg.ControllerName,
		renderCh:       cfg.RenderCh,
		controllerCh:   make(chan event.Event, 5),
		updaterCh:      make(chan event.Event, 5),
		logger:         cfg.Logger,
	}
}

func (o *Operator) Start(ctx context.Context) error {
	log := o.logger.WithName("operator")
	o.log = log
	o.ctx = ctx

	options := manager.Options{
		Scheme: scheme,
	}

	log.V(3).Info("obtaining cluster configuration")
	clusterCfg := ctlr.GetConfigOrDie()
	clusterCfg.Timeout = clusterTimeout

	log.V(3).Info("starting manager")
	mgr, err := manager.New(clusterCfg, options)
	if err != nil {
		return fmt.Errorf("cannot build runtime manager: %w", err)
	}

	log.V(3).Info("starting GatewayClass controller")
	o.SetupStore()

	err = stunnerctrl.RegisterGatewayClassController(mgr, o.gatewayClassStore, o.controllerName)
	if err != nil {
		return fmt.Errorf("cannot register gateway-class controller: %w", err)
	}

	err = stunnerctrl.RegisterGatewayConfigController(mgr, o.gatewayConfigStore, o.controllerCh)
	if err != nil {
		return fmt.Errorf("cannot register gatewayconfig controller: %w", err)
	}

	err = stunnerctrl.RegisterGatewayController(mgr, o.gatewayStore, o.controllerCh)
	if err != nil {
		return fmt.Errorf("cannot register gateway controller: %w", err)
	}

	err = stunnerctrl.RegisterUDPRouteController(mgr, o.udpRouteStore, o.controllerCh)
	if err != nil {
		return fmt.Errorf("cannot register udproute controller: %w", err)
	}

	err = stunnerctrl.RegisterServiceController(mgr, o.serviceStore, o.controllerCh)
	if err != nil {
		return fmt.Errorf("cannot register service controller: %w", err)
	}

	o.updater = updater.NewUpdater(updater.UpdaterConfig{
		Manager: mgr,
		Logger:  o.logger.WithName("updater"),
	})
	err = o.updater.Start(ctx)
	if err != nil {
		return fmt.Errorf("cannot spawn updater thread: %w", err)
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
	defer close(o.renderCh)
	defer close(o.controllerCh)

	for {
		select {
		case e := <-o.renderCh:
			if err := o.ProcessEvent(e, "render"); err != nil {
				o.log.Error(err, "could not process render event",
					"event", e.String())
			}

		case e := <-o.controllerCh:
			if err := o.ProcessEvent(e, "controller"); err != nil {
				o.log.Error(err, "could not process controller event",
					"event", e.String())
			}

		case <-ctx.Done():
			return
		}
	}
}

// ProcessEvent dispatches an event to the corresponding renderer
func (o *Operator) ProcessEvent(e event.Event, source string) error {
	switch e.GetType() {
	case event.EventTypeRender:
		// make sure we have received this from a controller
		if source != "controller" {
			return fmt.Errorf("render event received from the wrong source: %q",
				e.String())
		}

		// pass through to the renderer
		o.renderCh <- e
	case event.EventTypeUpdate:
		// make sure we have received this from a controller
		if source != "render" {
			return fmt.Errorf("update event received from the wrong source: %q",
				e.String())
		}

		// pass through to the updater
		o.updaterCh <- e
	}

	return nil
}
