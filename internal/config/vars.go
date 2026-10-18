// Package config allows to override some of the default settings from the exported default config
// package.
package config

import (
	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// -----------------------------------------------------------------------------
// Gateway - Consts
// -----------------------------------------------------------------------------

var (
	// ControllerName is the current controller name which indicates this operator's name
	ControllerName = opdefault.DefaultControllerName

	// EnableRelayToClusterIP allows clients to create transport relay connections directly to
	// the ClusterIP of a Kubernetes service. This is useful for hiding the pod IPs behind the
	// ClusterIP. Clients can then connect to both the ClusterIP and any direct pod IP.
	EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP

	// ThrottleTimeout defines the amount of time to wait before initiating a new config render
	// process. This allows to rate-limit config renders in very large clusters or frequently
	// changing resources, where the config rendering process is too expensive to be run after
	// every CRUD operation on the object being watched by the operator. The larger the
	// throttle timeout the slower the controller and the smaller the operator CPU
	// consumption. Default is 250 msec.
	ThrottleTimeout = opdefault.DefaultThrottleTimeout

	// StartupRenderTimeout bounds the startup gate: a freshly elected operator holds its first
	// render until every controller reported one reconcile, or until this much time passed.
	StartupRenderTimeout = opdefault.DefaultStartupRenderTimeout

	// ConfigDiscoveryAddress is the default URI at which config discovery requests are served.
	ConfigDiscoveryAddress = stnrapiv2.DefaultConfigDiscoveryAddress

	// EnableFinalizer is a global config to switch operator finalization on. The finalizer
	// will clean up all allocaeted Kubernetes resources (like dataplane deployments and
	// LoadBalancer Services) on exit and invalidate Gateway API resource statuses. Use with
	// caution: enabling this will caluse client connections to break on operator restart.
	EnableFinalizer = opdefault.DefaultEnableFinalizer

	// LabelFilter is the list of label keys that are stripped from a Gateway's label set
	// before propagation to the Deployment that the operator creates for that Gateway.
	// Override via the STUNNER_GATEWAY_OPERATOR_LABEL_FILTER env-var (comma-separated). If
	// the env-var is set, it replaces the default rather than extending it.
	LabelFilter = append([]string(nil), opdefault.DefaultLabelFilter...)

	// TURNRouteServed is true if the cluster serves the optional TURNRoute CRD.
	TURNRouteServed = false
)
