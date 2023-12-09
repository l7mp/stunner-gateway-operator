// Package config allows to override some of the default settings from the exported default config
// package.
package config

import (
	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"

	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// -----------------------------------------------------------------------------
// Gateway - Consts
// -----------------------------------------------------------------------------

var (
	// ControllerName is the current controller name which indicates this operator's name
	ControllerName = opdefault.DefaultControllerName

	// ConfigMapName names a ConfigMap the operator renders the stunnerd config file into
	ConfigMapName = opdefault.DefaultConfigMapName

	// EnableEndpointDiscovery enables EDS for finding the UDP-route backend endpoints
	EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery

	// EnableRelayToClusterIP allows clients to create transport relay connections directly to
	// the ClusterIP of a Kubernetes service. This is useful for hiding the pod IPs behind the
	// ClusterIP. If both EnableEndpointDiscovery and EnableRelayToClusterIP are on, clients
	// can connect to both the ClusterIP and any direct pod IP.
	EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP

	// ThrottleTimeout defines the amount of time to wait before initiating a new config render
	// process. This allows to rate-limit config renders in very large clusters or frequently
	// changing resources, where the config rendering process is too expensive to be run after
	// every CRUD operation on the object being watched by the operator. The larger the
	// throttle timeout the slower the controller and the smaller the operator CPU
	// consumption. Default is 250 msec.
	ThrottleTimeout = opdefault.DefaultThrottleTimeout

	// DataplaneMode is the "managed dataplane" mode. When set to "managed", the operator takes
	// care of providing the stunnerd pods for each Gateway. In "legacy" mode, the dataplanes
	// must be provided by the user.
	DataplaneMode = NewDataplaneMode(opdefault.DefaultDataplaneMode)

	// ConfigDiscoveryAddress is the default URI at which config discovery requests are served.
	ConfigDiscoveryAddress = stnrv1.DefaultConfigDiscoveryAddress
)
