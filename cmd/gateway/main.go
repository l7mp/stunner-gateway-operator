package main

import (
	"fmt"
	"os"

	flag "github.com/spf13/pflag"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/l7mp/stunner-kubernetes-gateway/internal/config"
	"github.com/l7mp/stunner-kubernetes-gateway/internal/manager"
)

const (
	domain string = "stunner.l7mp.io"
)

var (
	// Set during go build
	version string
	commit  string
	date    string

	// Command-line flags
	gatewayCtlrName = flag.String(
		"gateway-ctlr-name",
		"",
		fmt.Sprintf("The name of the Gateway controller. The controller name must be of the form: DOMAIN/NAMESPACE/NAME. The controller's domain is '%s'.", domain),
	)
)

func main() {
	flag.Parse()

	logger := zap.New()
	conf := config.Config{
		GatewayCtlrName: *gatewayCtlrName,
		Logger:          logger,
	}

	MustValidateArguments(
		flag.CommandLine,
		GatewayControllerParam(domain, "stunner-gateway" /* FIXME(f5yacobucci) dynamically set */),
	)

	logger.Info("Starting STUNner Kubernetes Gateway",
		"version", version,
		"commit", commit,
		"date", date)

	err := manager.Start(conf)
	if err != nil {
		logger.Error(err, "Failed to start control loop")
		os.Exit(1)
	}
}
