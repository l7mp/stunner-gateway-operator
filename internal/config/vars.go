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

	// EnableRenderThrottling makes is possible for the operator to queue up rendering requests
	// and collapsed them into a single request to decrease operator churn, default is true
	EnableRenderThrottling = DefaultEnableRenderThrottling
)
