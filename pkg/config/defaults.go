// Package "pkg/config" contaais the public API defaults and settings that may be reused across
// control plane projects.
package config

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	// DefaultControllerName is a unique identifier which indicates this operator's name.
	DefaultControllerName = "stunner.l7mp.io/gateway-operator"

	// RelatedGatewayAnnotationKey is the name of the annotation that is used to tie a
	// LoadBalancer service and a STUNner dataplane ConfigMap to a Gateway. The value is either
	// a singular pair of a namespace and name when of the related Gateway (in the form
	// "namespace/name", mostly used for associating a LB Service to a Gateway) or
	// GatewayConfig (used for ConfigMaps storing STUNner dataplane configs, which usually
	// belong to multiple Gateways).
	RelatedGatewayAnnotationKey = "stunner.l7mp.io/related-gateway-name"

	// AppLabelKey defines the label used to mark the pods of the stunnerd Deployment.
	AppLabelKey = "app"

	// AppLabelValue defines the label value used to mark the pods of the stunnerd deployment.
	AppLabelValue = "stunner"

	// OwnedByLabelKey is the name of the label that is used to mark resources (Services,
	// ConfigMaps, and Deployments) dynamically created and maintained by the operator. Note
	// that the Deployments and Services created by the operator will have both the AppLabelKey
	// and the OwnedByLabelKey labels set.
	OwnedByLabelKey = "stunner.l7mp.io/owned-by"

	// OwnedByLabelValue is the value of OwnedByLabelKey to indicate that a resource is
	// maintained by the operator.
	OwnedByLabelValue = "stunner"

	// ServiceTypeAnnotationKey defines the type of the service created to expose each Gateway
	// to external clients. Can be either `None` (no service created), `ClusterIP`, `NodePort`,
	// `ExternalName` or `LoadBalancer`. Default is `LoadBalancer`.
	ServiceTypeAnnotationKey = "stunner.l7mp.io/service-type"

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
)
