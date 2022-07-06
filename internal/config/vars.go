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
)
