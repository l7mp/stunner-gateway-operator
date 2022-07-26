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

	// EnableRenderThrottling makes is possible for the operator to queue up rendering requests
	// and collapsed them into a single request to decrease operator churn, default is true
	EnableRenderThrottling = DefaultEnableRenderThrottling
)
