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
	"strings"
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
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/renderer"
	"github.com/l7mp/stunner-gateway-operator/internal/updater"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

const (
	envVarMode    = "STUNNER_GATEWAY_OPERATOR_DATAPLANE_MODE"
	envVarAddress = "STUNNER_GATEWAY_OPERATOR_ADDRESS"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gwapiv1a2.AddToScheme(scheme))
	utilruntime.Must(gwapiv1.AddToScheme(scheme))
	utilruntime.Must(stnrgwv1a1.AddToScheme(scheme))
	utilruntime.Must(stnrgwv1.AddToScheme(scheme))
}

func main() {
	var controllerName, dataplaneMode, metricsAddr, cdsAddr, throttleTimeout, probeAddr string
	var enableLeaderElection, enableEDS bool

	flag.StringVar(&controllerName, "controller-name", opdefault.DefaultControllerName,
		"The conroller name to be used in the GatewayClass resource to bind it to this operator.")
	flag.StringVar(&throttleTimeout, "throttle-timeout", opdefault.DefaultThrottleTimeout.String(),
		"Time interval to wait between subsequent config renders.")
	flag.BoolVar(&enableEDS, "endpoint-discovery", opdefault.DefaultEnableEndpointDiscovery,
		fmt.Sprintf("Enable endpoint discovery, default: %t.", opdefault.DefaultEnableEndpointDiscovery))
	flag.StringVar(&dataplaneMode, "dataplane-mode", opdefault.DefaultDataplaneMode,
		`Managed dataplane mode: either "managed" (automatic dataplane provisioning using the config discovery service) or "legacy" (dataplane(s) provided by the user).`)
	flag.StringVar(&cdsAddr, "config-discovery-address", stnrv1.DefaultConfigDiscoveryAddress, `Config discovery server endpoint.`)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development:     true,
		DestWriter:      os.Stderr,
		StacktraceLevel: zapcore.Level(3),
		TimeEncoder:     zapcore.RFC3339NanoTimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger.WithName("ctrl-runtime"))
	setupLog := logger.WithName("setup")

	config.EnableEndpointDiscovery = enableEDS
	setupLog.Info("endpoint discovery", "state", enableEDS)

	if dataplaneMode == opdefault.DefaultDataplaneMode {
		// dataplane mode not overrridden on the command line: use env var
		envMode, ok := os.LookupEnv(envVarMode)
		if ok {
			dataplaneMode = envMode
		}
	}

	config.DataplaneMode = config.NewDataplaneMode(dataplaneMode)
	setupLog.Info("dataplane mode", "mode", config.DataplaneMode.String())

	if cdsAddr == stnrv1.DefaultConfigDiscoveryAddress {
		// CDS address not overrridden on the command line: use env var
		envAddr, ok := os.LookupEnv(envVarAddress)
		if ok {
			cdsAddr = envAddr
		}
		// add the default port
		as := strings.Split(cdsAddr, ":")
		if len(as) == 1 || (len(as) == 2 && as[1] == "") {
			dd := strings.Split(stnrv1.DefaultConfigDiscoveryAddress, ":")
			cdsAddr = fmt.Sprintf("%s:%s", cdsAddr, dd[1])
		}
	}
	config.ConfigDiscoveryAddress = cdsAddr
	setupLog.Info("config discovery server", "addr", config.ConfigDiscoveryAddress)

	if d, err := time.ParseDuration(throttleTimeout); err != nil {
		setupLog.Info("setting rate-limiting (throttle timeout)", "timeout", throttleTimeout)
		config.ThrottleTimeout = d
	}

	setupLog.Info("setting up Kubernetes controller manager")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
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

	setupLog.Info("setting up CDS server", "address", cdsAddr)
	c := config.NewCDSServer(config.ConfigDiscoveryAddress, logger)

	setupLog.Info("setting up operator")
	op := operator.NewOperator(operator.OperatorConfig{
		ControllerName: controllerName,
		Manager:        mgr,
		RenderCh:       r.GetRenderChannel(),
		ConfigCh:       c.GetConfigUpdateChannel(),
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
	if err := u.Start(ctx); err != nil {
		setupLog.Error(err, "could not run updater")
		os.Exit(1)
	}

	setupLog.Info("starting config discovery server")
	if err := c.Start(ctx); err != nil {
		setupLog.Error(err, "could not run config discovery server")
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
