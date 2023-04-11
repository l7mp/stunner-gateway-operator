package config

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultControllerName is a unique identifier which indicates this operator's name.
	DefaultControllerName = "stunner.l7mp.io/gateway-operator"

	// DefaultRelatedGatewayAnnotationKey is the name of the annotation that is used to tie a
	// LoadBalancer service and a STUNner dataplane ConfigMap to a Gateway. The value is either
	// a singular pair of a namespace and name when of the related Gateway (in the form
	// "namespace/name", mostly used for associating a LB Service to a Gateway) or
	// GatewayConfig (used for ConfigMaps storing STUNner dataplane configs, which usually
	// belong to multiple Gateways).
	DefaultRelatedGatewayAnnotationKey = "stunner.l7mp.io/related-gateway-name"

	// DefaultServiceTypeAnnotationKey defines the type of the service created to expose each Gateway
	// to external clients. Can be either `None` (no service created), `ClusterIP`, `NodePort`,
	// `ExternalName` or `LoadBalancer`. Default is `LoadBalancer`.
	DefaultServiceTypeAnnotationKey = "stunner.l7mp.io/service-type"

	// DefaultServiceType defines the default type of services created to expose each Gateway
	// to external clients.
	DefaultServiceType = corev1.ServiceTypeLoadBalancer

	// // GatewayManagedLabelValue indicates that the object's lifecycle is managed by
	// // the gateway controller.
	// GatewayManagedLabelValue = "gateway"

	// DefaultStunnerConfigMapName names a ConfigMap by the operator to render the stunnerd
	// config file.
	DefaultConfigMapName = "stunnerd-config"

	// DefaultStunnerdInstanceName specifies the name of the stunnerd instance managed by the
	// operator.
	DefaultStunnerdInstanceName = "stunner-daemon"

	// DefaultStunnerdConfigfileName defines the file name under which the generated configfile
	// will appear in the filesystem of the stunnerd pods. This is also the key on the
	// ConfigMap that maintains the stunnerd config.
	DefaultStunnerdConfigfileName = "stunnerd.conf"

	// DefaultAppLabelKey defines the label used to mark the stunnerd deployment.
	DefaultAppLabelKey = "app"

	// DefaultAppLabelValue defines the label value used to mark the stunnerd deployment.
	// FIXME make this configurable.
	DefaultAppLabelValue = "stunner"

	// DefaultEnableEndpointDiscovery enables EDS for finding the UDP-route backend endpoints.
	DefaultEnableEndpointDiscovery = true

	// EnableRelayToClusterIP allows clients to create transport relay connections to the
	// ClusterIP of a service.
	DefaultEnableRelayToClusterIP = true

	// DefaultHealthCheckEndpoint is the default URI at which health-check requests are served.
	DefaultHealthCheckEndpoint = "http://0.0.0.0:8086"

	// DefaultThrottleTimeout is the default time interval to wait between subsequent config
	// renders.
	DefaultThrottleTimeout = 250 * time.Millisecond

	// DefaultEnableHealthCheck enables health check port to be exposed.
	DefaultEnableHealthCheck = false

	// DefaultHealthCheckAnnotationKey is the name of the annotation that is used to
	// enable health check.
	DefaultHealthCheckAnnotationKey = "stunner.l7mp.io/expose-health-check-port"

	// Digitalocean requires exposed health check port for load balancers.
	// DefaultDoHealthCheckPortAnnotationKey is the name of the annotation that is used to
	// expose health check port
	DefaultDoHealthCheckPortAnnotationKey = "service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"

	// Digitalocean requires exposed health check port for load balancers.
	// DefaultDoHealthCheckPortAnnotationKey is the name of the annotation that is used to
	// expose health check by it's protocol
	DefaultDoHealthCheckProtocolAnnotationKey = "service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"
)
