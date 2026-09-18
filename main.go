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

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/l7mp/stunner/v2/pkg/buildinfo"

	"github.com/l7mp/stunner-gateway-operator/internal/app"
)

var (
	version    = "dev"
	commitHash = "n/a"
	buildDate  = "<unknown>"
)

func main() {
	cfg := app.NewConfig()
	cfg.BindFlags(flag.CommandLine, app.OSLookupEnv)

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

	buildInfo := buildinfo.BuildInfo{Version: version, CommitHash: commitHash, BuildDate: buildDate}
	setupLog.Info(fmt.Sprintf("starting STUNner gateway operator %s", buildInfo.String()))

	if err := cfg.Complete(app.OSLookupEnv); err != nil {
		setupLog.Error(err, "invalid configuration")
		os.Exit(1)
	}
	setupLog.Info("operator configuration", cfg.Summary()...)

	a, err := app.New(cfg, ctrl.GetConfigOrDie(), app.NewScheme(), logger)
	if err != nil {
		setupLog.Error(err, "unable to set up the operator")
		os.Exit(1)
	}

	setupLog.Info("starting the operator")
	if err := a.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running the operator")
		// no way to gracefully terminate: give up and exit with an error
		os.Exit(1)
	}
}
