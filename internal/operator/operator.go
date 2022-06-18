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
	ctx                context.Context
	controllerName     string
	gatewayClassStore  store.Store
	gatewayConfigStore store.Store
	gatewayStore       store.Store
	udpRouteStore      store.Store
	serviceStore       store.Store
	renderCh           chan event.Event
	manager            manager.Manager
	log, logger        logr.Logger
}

// NewOperator creates a new Operator
func NewOperator(cfg OperatorConfig) *Operator {
	return &Operator{
		controllerName: cfg.ControllerName,
		renderCh:       cfg.RenderCh,
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

	err = stunnerctrl.RegisterGatewayConfigController(mgr, o.gatewayConfigStore, o.renderCh)
	if err != nil {
		return fmt.Errorf("cannot register gatewayconfig controller: %w", err)
	}

	err = stunnerctrl.RegisterGatewayController(mgr, o.gatewayStore, o.renderCh)
	if err != nil {
		return fmt.Errorf("cannot register gateway controller: %w", err)
	}

	err = stunnerctrl.RegisterUDPRouteController(mgr, o.udpRouteStore, o.renderCh)
	if err != nil {
		return fmt.Errorf("cannot register udproute controller: %w", err)
	}

	err = stunnerctrl.RegisterServiceController(mgr, o.serviceStore, o.renderCh)
	if err != nil {
		return fmt.Errorf("cannot register service controller: %w", err)
	}

	return nil
}

func (o *Operator) SetupStore() {
	o.gatewayClassStore = store.NewStore(o.logger.WithName("gateway-class-store"))
	o.gatewayConfigStore = store.NewStore(o.logger.WithName("gatewayconfig-store"))
	o.gatewayStore = store.NewStore(o.logger.WithName("gateway-store"))
	o.udpRouteStore = store.NewStore(o.logger.WithName("udproute-store"))
	o.serviceStore = store.NewStore(o.logger.WithName("service-store"))
}
