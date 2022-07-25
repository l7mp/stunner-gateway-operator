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

	// EnableRelayToClusterIP allows clients to create transport relay connections to the
	// ClusterIP of a Kubernetes serviceThis is useful for hiding the pod IPs behind the
	// ClusterIP. If both EnableEndpointDiscovery and EnableRelayToClusterIP is on, clients
	// connect both via the ClusterIP and the direct pod IP.
	EnableRelayToClusterIP = DefaultEnableRelayToClusterIP

	// EnableRenderThrottling makes is possible for the operator to queue up rendering requests
	// and collapsed them into a single request to decrease operator churn, default is true
	EnableRenderThrottling = DefaultEnableRenderThrottling
)
