package operator

// -----------------------------------------------------------------------------
// Gateway - Consts
// -----------------------------------------------------------------------------

const (
	// DefaultControllerName is a unique identifier which indicates this operator's name
	DefaultControllerName = "stunner.l7mp.io/gateway-operator"

	// GatewayAddressAnnotationKey is the name of the annotation that is used to tie a
	// LoadBalancer service to the Gateway. If a STUNner deployment exposes multiple listeners
	// (e.g., one on TCP and another on UDP) wrapped by different LoadBalancer services, each
	// with a distinct External IP, then each listener must go to a separate Gateway resource
	// so that the controller can assign the right public  address to the right listener
	GatewayAddressAnnotationKey = "stunner.l7mp.io/related-gateway-name"

	// // GatewayManagedLabelValue indicates that the object's lifecycle is managed by
	// // the gateway controller.
	// GatewayManagedLabelValue = "gateway"

	// DefaultStunnerConfigMapName names a ConfigMap by the operator to render the stunnerd config file
	DefaultConfigMapName = "stunnerd-config"

	// DefaultStunnerdInstanceName specifies the name of the stunnerd instance managed by the operator
	DefaultStunnerdInstanceName = "stunner-daemon"
)
