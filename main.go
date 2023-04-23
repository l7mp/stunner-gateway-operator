/*
Copyright 2022 The l7mp/stunner team.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	//+kubebuilder:scaffold:imports

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1alpha2.AddToScheme(scheme))
	utilruntime.Must(stunnerv1alpha1.AddToScheme(scheme))
}

func main() {
	var controllerName, metricsAddr, throttleTimeout, probeAddr string
	var enableLeaderElection, enableEDS bool

	flag.StringVar(&controllerName, "controller-name", opdefault.DefaultControllerName,
		"The conroller name to be used in the GatewayClass resource to bind it to this operator.")
	flag.StringVar(&throttleTimeout, "throttle-timeout", opdefault.DefaultThrottleTimeout.String(),
		"Time interval to wait between subsequent config renders.")
	flag.BoolVar(&enableEDS, "endpoint-discovery", opdefault.DefaultEnableEndpointDiscovery,
		fmt.Sprintf("Enable endpoint discovery, default: %t.", opdefault.DefaultEnableEndpointDiscovery))
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts), func(o *zap.Options) {
		o.TimeEncoder = zapcore.RFC3339NanoTimeEncoder
	})
	ctrl.SetLogger(logger.WithName("ctrl-runtime"))
	setupLog := logger.WithName("setup")

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts), func(o *zap.Options) {
		o.TimeEncoder = zapcore.RFC3339NanoTimeEncoder
	}))

	setupLog.Info("endpoint discovery", "state", enableEDS)
	config.EnableEndpointDiscovery = enableEDS

	if d, err := time.ParseDuration(throttleTimeout); err != nil {
		setupLog.Info("setting rate-limiting (throttle timeout)", "timeout", throttleTimeout)
		config.ThrottleTimeout = d
	}

	setupLog.Info("setting up Kubernetes controller manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "92062b70.l7mp.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to set up Kubernetes controller manager")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("setting up STUNner config renderer")
	r := renderer.NewRenderer(renderer.RendererConfig{
		Scheme: scheme,
		Logger: logger,
	})

	setupLog.Info("setting up updater client")
	u := updater.NewUpdater(updater.UpdaterConfig{
		Manager: mgr,
		Logger:  logger,
	})

	setupLog.Info("setting up operator")
	op := operator.NewOperator(operator.OperatorConfig{
		ControllerName: controllerName,
		Manager:        mgr,
		RenderCh:       r.GetRenderChannel(),
		UpdaterCh:      u.GetUpdaterChannel(),
		Logger:         logger,
	})

	r.SetOperatorChannel(op.GetOperatorChannel())

	ctx := ctrl.SetupSignalHandler()

	setupLog.Info("starting renderer thread")
	if err := r.Start(ctx); err != nil {
		setupLog.Error(err, "problem running renderer")
		os.Exit(1)
	}

	setupLog.Info("starting updater thread")
	err = u.Start(ctx)
	if err != nil {
		setupLog.Error(err, "problem running updater")
		os.Exit(1)
	}

	setupLog.Info("starting operator thread")
	if err := op.Start(ctx); err != nil {
		setupLog.Error(err, "problem running operator")
		os.Exit(1)
	}

	setupLog.Info("starting Kubernetes controller manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
