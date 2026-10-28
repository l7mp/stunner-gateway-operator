package app

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/config"
	licensemgr "github.com/l7mp/stunner-gateway-operator/v2/internal/licensemanager"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/updater"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"
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

	config.EnableFinalizer = cfg.EnableFinalizer
	config.ConfigDiscoveryAddress = cfg.CDSAdvertisedAddress
	config.LabelFilter = append([]string(nil), cfg.LabelFilter...)
	config.ThrottleTimeout = cfg.ThrottleTimeout
	config.StartupRenderTimeout = cfg.StartupRenderTimeout

	log.Info("setting up Kubernetes controller manager")
	ownedBy := labels.SelectorFromSet(labels.Set{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue})
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
		// the cache holds no managed fields and only the Deployments and DaemonSets the
		// operator owns; Secrets are watched by their metadata only and read from the API server
		Cache: cache.Options{
			DefaultTransform: cache.TransformStripManagedFields(),
			ByObject: map[client.Object]cache.ByObject{
				&appv1.Deployment{}: {Label: ownedBy},
				&appv1.DaemonSet{}:  {Label: ownedBy},
			},
		},
		Client: client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}},
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
