package manager

import (
	"fmt"
	"time"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctlr "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-kubernetes-gateway/internal/config"
	"github.com/l7mp/stunner-kubernetes-gateway/internal/events"
	hr "github.com/l7mp/stunner-kubernetes-gateway/internal/implementations/httproute"
	svc "github.com/l7mp/stunner-kubernetes-gateway/internal/implementations/service"
	stnrcfg "github.com/l7mp/stunner-kubernetes-gateway/internal/stunner/config"
	"github.com/l7mp/stunner-kubernetes-gateway/internal/stunner/file"
	stnrruntime "github.com/l7mp/stunner-kubernetes-gateway/internal/stunner/runtime"
	"github.com/l7mp/stunner-kubernetes-gateway/internal/state"
	"github.com/l7mp/stunner-kubernetes-gateway/internal/status"
	"github.com/l7mp/stunner-kubernetes-gateway/pkg/sdk"
)

// clusterTimeout is a timeout for connections to the Kubernetes API
const clusterTimeout = 10 * time.Second

var scheme = runtime.NewScheme()

func init() {
	// FIXME(pleshakov): handle errors returned by the calls bellow
	_ = gatewayv1alpha2.AddToScheme(scheme)
	_ = apiv1.AddToScheme(scheme)
}

func Start(cfg config.Config) error {
	logger := cfg.Logger

	options := manager.Options{
		Scheme: scheme,
	}

	eventCh := make(chan interface{})

	clusterCfg := ctlr.GetConfigOrDie()
	clusterCfg.Timeout = clusterTimeout

	mgr, err := manager.New(clusterCfg, options)
	if err != nil {
		return fmt.Errorf("cannot build runtime manager: %w", err)
	}

	err = sdk.RegisterHTTPRouteController(mgr, hr.NewHTTPRouteImplementation(cfg, eventCh))
	if err != nil {
		return fmt.Errorf("cannot register httproute implementation: %w", err)
	}
	err = sdk.RegisterServiceController(mgr, svc.NewServiceImplementation(cfg, eventCh))
	if err != nil {
		return fmt.Errorf("cannot register service implementation: %w", err)
	}

	conf := state.NewConfiguration(cfg.GatewayCtlrName, state.NewRealClock())
	serviceStore := state.NewServiceStore()
	reporter := status.NewUpdater(mgr.GetClient(), cfg.Logger)
	configGenerator := stnrcfg.NewGeneratorImpl(serviceStore)
	stunnerFileMgr := file.NewManagerImpl()
	stunnerRuntimeMgr := stnrruntime.NewManagerImpl()
	eventLoop := events.NewEventLoop(conf, serviceStore, configGenerator, eventCh, reporter, cfg.Logger, stunnerFileMgr, stunnerRuntimeMgr)

	err = mgr.Add(eventLoop)
	if err != nil {
		return fmt.Errorf("cannot register event loop: %w", err)
	}

	ctx := ctlr.SetupSignalHandler()

	logger.Info("Starting manager")
	return mgr.Start(ctx)
}
