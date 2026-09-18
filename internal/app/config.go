// Package app assembles the operator: it parses the configuration from the command line and
// the environment, builds the controller manager and every subsystem, and registers them as
// manager runnables. Both the binary and the integration tests start the operator through it.
package app

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	stnrv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"

	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// Environment variables that override the defaults or the flags.
const (
	EnvVarMode           = "STUNNER_GATEWAY_OPERATOR_DATAPLANE_MODE"
	EnvVarAddress        = "STUNNER_GATEWAY_OPERATOR_ADDRESS"
	EnvVarControllerName = "STUNNER_GATEWAY_OPERATOR_CONTROLLER_NAME"
	EnvVarPprofAddr      = "STUNNER_GATEWAY_OPERATOR_PPROF_BIND_ADDRESS"
	EnvVarLabelFilter    = "STUNNER_GATEWAY_OPERATOR_LABEL_FILTER"
	EnvVarCustomerKey    = "CUSTOMER_KEY"
)

// LookupEnv is the shape of os.LookupEnv, so tests can supply their own environment.
type LookupEnv func(key string) (string, bool)

// Config is the complete operator configuration.
type Config struct {
	// ControllerName is the controller name bound to the GatewayClass resources.
	ControllerName string
	// DataplaneMode is "managed" or "legacy".
	DataplaneMode string
	// EnableEndpointDiscovery enables EDS for the UDPRoute backends.
	EnableEndpointDiscovery bool
	// DisableEndpointSliceController falls back to the legacy Endpoints controller.
	DisableEndpointSliceController bool
	// EnableFinalizer cleans up allocated resources and invalidates statuses on exit. It is
	// incompatible with leader election.
	EnableFinalizer bool
	// ThrottleTimeout is the interval between config renders.
	ThrottleTimeout time.Duration
	// StartupRenderTimeout bounds the startup gate that holds the first render until every
	// controller reported.
	StartupRenderTimeout time.Duration
	// LabelFilter lists the Gateway label keys that are not propagated to the dataplane.
	LabelFilter []string

	// CDSAddress is the address the config discovery server listens on.
	CDSAddress string
	// CDSAdvertisedAddress is the config discovery server address handed to the dataplane
	// pods. Complete derives it from the EnvVarAddress host and the CDSAddress port.
	CDSAdvertisedAddress string

	// MetricsAddr, ProbeAddr and PprofAddr are the bind addresses of the manager's HTTP
	// servers. PprofAddr "0" disables profiling.
	MetricsAddr string
	ProbeAddr   string
	PprofAddr   string

	// LeaderElection enables leader election among the operator replicas.
	LeaderElection bool
	// LeaderElectionID names the Lease the replicas compete for.
	LeaderElectionID string
	// LeaderElectionNamespace is where the Lease lives; empty means the in-cluster namespace.
	LeaderElectionNamespace string
	// LeaseDuration, RenewDeadline and RetryPeriod override the controller-runtime defaults
	// when set. They are not exposed as flags.
	LeaseDuration, RenewDeadline, RetryPeriod *time.Duration

	// CustomerKey unlocks the licensed features.
	CustomerKey string

	// SkipNameValidation lets several managers register controllers of the same names in one
	// process. Tests only.
	SkipNameValidation bool

	// flags is the flag set BindFlags registered on, so that Complete can tell an explicitly
	// set flag from its default.
	flags *flag.FlagSet
}

// NewConfig returns the default configuration.
func NewConfig() Config {
	return Config{
		ControllerName:          opdefault.DefaultControllerName,
		DataplaneMode:           opdefault.DefaultDataplaneMode,
		EnableEndpointDiscovery: opdefault.DefaultEnableEndpointDiscovery,
		EnableFinalizer:         opdefault.DefaultEnableFinalizer,
		ThrottleTimeout:         opdefault.DefaultThrottleTimeout,
		StartupRenderTimeout:    opdefault.DefaultStartupRenderTimeout,
		LabelFilter:             append([]string(nil), opdefault.DefaultLabelFilter...),
		CDSAddress:              stnrv1.DefaultConfigDiscoveryAddress,
		MetricsAddr:             ":8080",
		ProbeAddr:               ":8081",
		PprofAddr:               "0",
		LeaderElectionID:        opdefault.DefaultLeaderElectionID,
	}
}

// BindFlags registers the command line flags on fs. The controller name default comes from the
// environment, so that the flag help shows the effective default.
func (c *Config) BindFlags(fs *flag.FlagSet, lookup LookupEnv) {
	c.flags = fs
	if name, ok := lookup(EnvVarControllerName); ok {
		c.ControllerName = name
	}

	fs.StringVar(&c.ControllerName, "controller-name", c.ControllerName,
		"The conroller name to be used in the GatewayClass resource to bind it to this operator.")
	fs.DurationVar(&c.ThrottleTimeout, "throttle-timeout", c.ThrottleTimeout,
		"Time interval to wait between subsequent config renders.")
	fs.DurationVar(&c.StartupRenderTimeout, "startup-render-timeout", c.StartupRenderTimeout,
		"Time to wait after becoming leader for every controller to report before the first render.")
	fs.BoolVar(&c.EnableEndpointDiscovery, "endpoint-discovery", c.EnableEndpointDiscovery,
		fmt.Sprintf("Enable endpoint discovery, default: %t.", c.EnableEndpointDiscovery))
	fs.StringVar(&c.DataplaneMode, "dataplane-mode", c.DataplaneMode,
		`Managed dataplane mode: either "managed" (automatic dataplane provisioning using the config discovery service) or "legacy" (dataplane(s) provided by the user).`)
	fs.StringVar(&c.CDSAddress, "config-discovery-address", c.CDSAddress, "Config discovery server endpoint.")
	fs.StringVar(&c.MetricsAddr, "metrics-bind-address", c.MetricsAddr, "The address the metric endpoint binds to.")
	fs.StringVar(&c.ProbeAddr, "health-probe-bind-address", c.ProbeAddr, "The address the probe endpoint binds to.")
	fs.StringVar(&c.PprofAddr, "pprof-bind-address", c.PprofAddr, "The address the pprof endpoint binds to. Set to \"0\" to disable.")
	fs.BoolVar(&c.DisableEndpointSliceController, "disable-endpontslice-controller", c.DisableEndpointSliceController,
		"Disable the EndpointSlice controller and fall back to the legacy Endpoints controller.")
	fs.BoolVar(&c.LeaderElection, "leader-elect", c.LeaderElection,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&c.LeaderElectionID, "leader-election-id", c.LeaderElectionID,
		"Name of the Lease the operator replicas compete for.")
	fs.StringVar(&c.LeaderElectionNamespace, "leader-election-namespace", c.LeaderElectionNamespace,
		"Namespace of the Lease; defaults to the namespace the operator runs in.")
	fs.BoolVar(&c.EnableFinalizer, "enable-finalizer", c.EnableFinalizer,
		"Clean up allocated resources and invalidate resource statuses on operator exit.")
}

// Complete applies the environment overrides that take effect after flag parsing, derives the
// advertised config discovery address and validates the result.
func (c *Config) Complete(lookup LookupEnv) error {
	// dataplane mode not set on the command line: use the env var
	if !c.flagSet("dataplane-mode") {
		if mode, ok := lookup(EnvVarMode); ok {
			c.DataplaneMode = mode
		}
	}

	if c.PprofAddr == "0" {
		if addr, ok := lookup(EnvVarPprofAddr); ok {
			c.PprofAddr = addr
		}
	}

	if key, ok := lookup(EnvVarCustomerKey); ok && key != "" {
		c.CustomerKey = key
	}

	// label filter: when set, the env var replaces the default; set-empty disables filtering
	if raw, ok := lookup(EnvVarLabelFilter); ok {
		c.LabelFilter = c.LabelFilter[:0]
		for _, k := range strings.Split(raw, ",") {
			if k = strings.TrimSpace(k); k != "" {
				c.LabelFilter = append(c.LabelFilter, k)
			}
		}
	}

	// the advertised address is the env var host with the configured port
	_, port, err := net.SplitHostPort(c.CDSAddress)
	if err != nil || port == "" {
		return fmt.Errorf("invalid config discovery server address %q", c.CDSAddress)
	}
	c.CDSAdvertisedAddress = c.CDSAddress
	if host, ok := lookup(EnvVarAddress); ok {
		c.CDSAdvertisedAddress = net.JoinHostPort(host, port)
	}

	if c.EnableFinalizer && c.LeaderElection {
		return fmt.Errorf("the finalizer cannot be enabled together with leader election: " +
			"finalization removes the resources a successor replica is about to take over")
	}

	return nil
}

// flagSet reports whether the named flag was given on the command line.
func (c *Config) flagSet(name string) bool {
	set := false
	if c.flags != nil {
		c.flags.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	}
	return set
}

// Summary lists the effective settings for the startup log.
func (c *Config) Summary() []any {
	return []any{
		"controller-name", c.ControllerName,
		"dataplane-mode", c.DataplaneMode,
		"endpoint-discovery", c.EnableEndpointDiscovery,
		"endpointslice-controller", !c.DisableEndpointSliceController,
		"finalizer", c.EnableFinalizer,
		"throttle-timeout", c.ThrottleTimeout.String(),
		"startup-render-timeout", c.StartupRenderTimeout.String(),
		"label-filter", c.LabelFilter,
		"cds-address", c.CDSAddress,
		"cds-advertised-address", c.CDSAdvertisedAddress,
		"leader-election", c.LeaderElection,
		"leader-election-id", c.LeaderElectionID,
		"pprof-address", c.PprofAddr,
		"customer-key", map[bool]string{true: "AVAILABLE", false: "MISSING"}[c.CustomerKey != ""],
	}
}

// OSLookupEnv adapts os.LookupEnv to LookupEnv.
var OSLookupEnv LookupEnv = os.LookupEnv
