package app

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	licensemgr "github.com/l7mp/stunner-gateway-operator/internal/licensemanager"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
)

// App is the assembled operator: the manager and the subsystems registered with it.
type App struct {
	Config         Config
	Manager        manager.Manager
	LicenseManager licensemgr.Manager
	Renderer       renderer.Renderer
	Updater        *updater.Updater
	CDSServer      *config.Server
	Operator       *operator.Operator
}

// NewScheme returns the runtime scheme with every API the operator watches or writes.
func NewScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme)) //nolint:staticcheck
	utilruntime.Must(gwapiv1a2.AddToScheme(scheme))      //nolint:staticcheck
	utilruntime.Must(gwapiv1.AddToScheme(scheme))        //nolint:staticcheck
	utilruntime.Must(stnrgwv1a1.AddToScheme(scheme))     //nolint:staticcheck
	utilruntime.Must(stnrgwv1.AddToScheme(scheme))       //nolint:staticcheck
	return scheme
}

// New builds the manager and every subsystem from cfg and registers them as manager runnables.
// The renderer, the updater, the config discovery server and the operator are leader-only; the
// license manager runs on every replica. Nothing runs until Start.
func New(cfg Config, restConfig *rest.Config, scheme *runtime.Scheme, logger logr.Logger) (*App, error) {
	log := logger.WithName("setup")

	config.EnableEndpointDiscovery = cfg.EnableEndpointDiscovery
	config.EndpointSliceAvailable = !cfg.DisableEndpointSliceController // controller may override this
	config.EnableFinalizer = cfg.EnableFinalizer
	config.DataplaneMode = config.NewDataplaneMode(cfg.DataplaneMode)
	config.ConfigDiscoveryAddress = cfg.CDSAdvertisedAddress
	config.LabelFilter = append([]string(nil), cfg.LabelFilter...)
	config.ThrottleTimeout = cfg.ThrottleTimeout
	config.StartupRenderTimeout = cfg.StartupRenderTimeout

	log.Info("setting up Kubernetes controller manager")
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.MetricsAddr,
		},
		HealthProbeBindAddress:        cfg.ProbeAddr,
		PprofBindAddress:              cfg.PprofAddr,
		LeaderElection:                cfg.LeaderElection,
		LeaderElectionID:              cfg.LeaderElectionID,
		LeaderElectionNamespace:       cfg.LeaderElectionNamespace,
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 cfg.LeaseDuration,
		RenewDeadline:                 cfg.RenewDeadline,
		RetryPeriod:                   cfg.RetryPeriod,
		Controller:                    ctrlcfg.Controller{SkipNameValidation: &cfg.SkipNameValidation},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to set up Kubernetes controller manager: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("unable to set up ready check: %w", err)
	}

	log.Info("setting up license manager")
	m := licensemgr.NewManager(cfg.CustomerKey, logger)

	log.Info("setting up config renderer")
	r := renderer.NewRenderer(renderer.RendererConfig{
		Scheme:         scheme,
		LicenseManager: m,
		Logger:         logger,
	})

	log.Info("setting up updater client")
	u := updater.NewUpdater(updater.UpdaterConfig{
		Manager: mgr,
		Logger:  logger,
	})

	log.Info("setting up config discovery server", "address", cfg.CDSAddress,
		"advertised-address", cfg.CDSAdvertisedAddress)
	c := config.NewCDSServer(cfg.CDSAddress, logger)

	log.Info("setting up operator")
	op, err := operator.NewOperator(operator.OperatorConfig{
		ControllerName: cfg.ControllerName,
		Manager:        mgr,
		LicenseManager: m,
		Renderer:       r,
		Updater:        u,
		CDSServer:      c,
		Logger:         logger,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to set up operator: %w", err)
	}

	for name, runnable := range map[string]manager.Runnable{
		"license-manager": m, "renderer": r, "updater": u, "config-discovery-server": c, "operator": op,
	} {
		if err := mgr.Add(runnable); err != nil {
			return nil, fmt.Errorf("unable to register the %s with the manager: %w", name, err)
		}
	}

	return &App{
		Config:         cfg,
		Manager:        mgr,
		LicenseManager: m,
		Renderer:       r,
		Updater:        u,
		CDSServer:      c,
		Operator:       op,
	}, nil
}

// Start runs the manager, and with it every subsystem, until the context ends. With leader
// election enabled the leader-only runnables start once the Lease is acquired, and Start
// returns an error when the Lease is lost, after which the process must exit.
func (a *App) Start(ctx context.Context) error {
	return a.Manager.Start(ctx)
}
