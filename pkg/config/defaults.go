// Package "pkg/config" contaais the public API defaults and settings that may be reused across
// control plane projects.
package config

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"
)

// Labeling rules
// - all top-level resources (Service, Deployment, ConfigMap) are labeled with "OwnedByLabelKey:OwnedByLabelValue"
// - additional mandatory labels are "RelatedGatewayKey:Gateway.GetName()" and "RelatedGatewayNamespace:Gateway.GetNamespace()"
// - Deployment pods are labeled with "AppLabelKey:AppLabelValue" and "RelatedGatewayKey:Gateway.GetName()" and "RelatedGatewayNamespace:Gateway.GetNamespace()"
// - all resources must have an owner-reference to a Gateway or a GatewayConfig (stunnerd-config ConfigMap in legacy dataplane mode) for the operator to pick them up

const (
	// DefaultControllerName is a unique identifier which indicates this operator's name.
	DefaultControllerName = "stunner.l7mp.io/gateway-operator"

	// DefaultDataplaneName is the name of the default Dataplane to use when no dataplane is specified explicitly.
	DefaultDataplaneName = "default"

	// DefaultDataplaneMode is the default dataplane mode.
	DefaultDataplaneMode = "managed"

	// DefaultEndpointSliceAvailable enables the EndpointSlice controller.
	DefaultEndpointSliceAvailable = true

	// OwnedByLabelKey is the name of the label that is used to mark resources (Services,
	// ConfigMaps, and Deployments) dynamically created and maintained by the operator. Note
	// that the Deployments and Services created by the operator will have both the AppLabelKey
	// and the OwnedByLabelKey labels set.
	OwnedByLabelKey = stnrconfv1.DefaultOwnedByLabelKey

	// OwnedByLabelValue is the value of OwnedByLabelKey to indicate that a resource is
	// maintained by the operator.
	OwnedByLabelValue = stnrconfv1.DefaultOwnedByLabelValue

	// RelatedGatewayKey is the name of the label that is used to tie a LoadBalancer service, a
	// STUNner dataplane ConfigMap, or a stunnerd Deployment (in managed mode) to a
	// Gateway. The value is either a singular pair of a namespace and name when of the related
	// Gateway (in the form "namespace/name", mostly used for associating a LB Service to a
	// Gateway) or GatewayConfig (used for ConfigMaps storing STUNner dataplane configs in
	// legacy mode, which usually belong to multiple Gateways).
	RelatedGatewayKey = stnrconfv1.DefaultRelatedGatewayKey

	// RelatedGatewayNamespace is the name of the label that is used to tie a LoadBalancer
	// service, a STUNner dataplane ConfigMap, or a stunnerd Deployment (in managed mode) to a
	// Gateway. The value is the namespace of the related Gateway.
	RelatedGatewayNamespace = stnrconfv1.DefaultRelatedGatewayNamespace

	// AppLabelKey defines the label used to mark the pods of the stunnerd Deployment.
	AppLabelKey = stnrconfv1.DefaultAppLabelKey

	// AppLabelValue defines the label value used to mark the pods of the stunnerd deployment.
	AppLabelValue = stnrconfv1.DefaultAppLabelValue

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

	// DefaultConfigMapName names a ConfigMap by the operator to render the stunnerd
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

	// DefaultEnableRelayToClusterIP allows clients to create transport relay connections to the
	// ClusterIP of a service.
	DefaultEnableRelayToClusterIP = true

	// DefaultThrottleTimeout is the default time interval to wait between subsequent config
	// renders.
	DefaultThrottleTimeout = 250 * time.Millisecond

	// DefaultMetricsPortName defines the name of the container-port used to expose the metrics
	// endpoint (if enabled).
	DefaultMetricsPortName = "metrics-port"

	// MixedProtocolAnnotationKey is the name(key) of the annotation that is used to
	// disable STUNner's blocking of mixed-protocol LBs for specific Gateways.
	// If false or any other string other than true the LB's proto defaults to the first
	// valid listener protocol in the Gateway spec.
	// If true all valid listener protocols will be added to the LB.
	MixedProtocolAnnotationKey = "stunner.l7mp.io/enable-mixed-protocol-lb"

	// MixedProtocolAnnotationValue is the expected value in order to enable mixed protocol LBs.
	MixedProtocolAnnotationValue = "true"

	// ExternalTrafficPolicyAnnotationKey is the name(key) of the annotation that is used to
	// control whether ExternalTrafficPolicy=Local is enabled on a LB Service that exposes a
	// Gateway, see https://github.com/l7mp/stunner-gateway-operator/issues/47.
	ExternalTrafficPolicyAnnotationKey = "stunner.l7mp.io/external-traffic-policy"

	// ExternalTrafficPolicyAnnotationValue is the expected value in order to
	// ExternalTrafficPolicy=Local.
	ExternalTrafficPolicyAnnotationValue = "local"

	// ManagedDataplaneDisabledAnnotationKey is the name(key) of the annotation that is used to
	// prevent the operator from creating a `stunnerd` dataplane Deployment for a Gateway.
	ManagedDataplaneDisabledAnnotationKey = "stunner.l7mp.io/disable-managed-dataplane"

	// ManagedDataplaneDisabledAnnotationValue is the value that can be used to disable the
	// managed dataplane feature for a Gateway.
	ManagedDataplaneDisabledAnnotationValue = "true"
)

var (
	// DefaultHealthCheckEndpoint is the default URI at which health-check requests are served.
	DefaultHealthCheckEndpoint = fmt.Sprintf("http://:%d", stnrconfv1.DefaultHealthCheckPort)

	// DefaultMetricsEndpoint is the default URI at which metrics scaping requests are served.
	DefaultMetricsEndpoint = fmt.Sprintf("http://:%d/metrics", stnrconfv1.DefaultMetricsPort)
)
