package config

// -----------------------------------------------------------------------------
// Gateway - Consts
// -----------------------------------------------------------------------------

var (
	// ControllerName is the current controller name which indicates this operator's name
	ControllerName = DefaultControllerName

	// ConfigMapName names a ConfigMap the operator renders the stunnerd config file into
	ConfigMapName = DefaultConfigMapName

	// EnableEndpointDiscovery enables EDS for finding the UDP-route backend endpoints
	EnableEndpointDiscovery = DefaultEnableEndpointDiscovery

	// EnableRelayToClusterIP allows clients to create transport relay connections directly to
	// the ClusterIP of a Kubernetes service. This is useful for hiding the pod IPs behind the
	// ClusterIP. If both EnableEndpointDiscovery and EnableRelayToClusterIP are on, clients
	// can connect to both the ClusterIP and any direct pod IP.
	EnableRelayToClusterIP = DefaultEnableRelayToClusterIP

	// ThrottleTimeout defines the amount of time to wait before initiating a new config render
	// process. This allows to rate-limit config renders in very large clusters or frequently
	// changing resources, where the config rendering process is too expensive to be run after
	// every CRUD operation on the object being watched by the operator. The larger the
	// throttle timeout the slower the controller and the smaller the operator CPU
	// consumption. Default is 250 msec.
	ThrottleTimeout = DefaultThrottleTimeout
)
