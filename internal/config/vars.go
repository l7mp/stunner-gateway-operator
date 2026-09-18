// Package config allows to override some of the default settings from the exported default config
// package.
package config

import (
	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"

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

	// StartupRenderTimeout bounds the startup gate: a freshly elected operator holds its first
	// render until every controller reported one reconcile, or until this much time passed.
	StartupRenderTimeout = opdefault.DefaultStartupRenderTimeout

	// DataplaneMode is the "managed dataplane" mode. When set to "managed", the operator takes
	// care of providing the stunnerd pods for each Gateway. In "legacy" mode, the dataplanes
	// must be provided by the user.
	DataplaneMode = NewDataplaneMode(opdefault.DefaultDataplaneMode)

	// ConfigDiscoveryAddress is the default URI at which config discovery requests are served.
	ConfigDiscoveryAddress = stnrapiv1.DefaultConfigDiscoveryAddress

	// EndpointSliceAvailable is a global flag indicating whether EndpointSlices are available
	// in the current cluster. This is detected in the UDPRoute controller trying to create a
	// Watch for EndpointSlices. If successful, only EndpointSlices will be considered and
	// Endpoints support is disables. Otherwise, the opetator falls back to Endoints support
	// (note that this breaks graceful backend shutdown, see
	// https://github.com/l7mp/stunner/issues/138).
	EndpointSliceAvailable = opdefault.DefaultEndpointSliceAvailable

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

	// GwAPIUDPRouteVersion is the API version at which the cluster serves the official
	// Gateway API UDPRoute resource: the graduated "v1" version is preferred over the
	// deprecated "v1alpha2". The route controller detects the served version at startup;
	// an empty value means the CRD is not installed. Route statuses are written back at
	// this version.
	GwAPIUDPRouteVersion = GwAPIVersionUnavailable

	// GwAPITCPRouteVersion is the API version at which the cluster serves the official
	// Gateway API TCPRoute resource (see GwAPIUDPRouteVersion).
	GwAPITCPRouteVersion = GwAPIVersionUnavailable
)

// Gateway API route resource version markers.
const (
	// GwAPIVersionUnavailable means the resource is not served by the cluster.
	GwAPIVersionUnavailable = ""
	// GwAPIVersionV1 is the graduated v1 version.
	GwAPIVersionV1 = "v1"
	// GwAPIVersionV1A2 is the deprecated v1alpha2 version.
	GwAPIVersionV1A2 = "v1alpha2"
)
