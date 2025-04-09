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

	// DefaultEnableFinalizer controls whether to enable the operator finalizer.
	DefaultEnableFinalizer = false

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

	// Annotations

	// MixedProtocolAnnotationKey is the name(key) of the Gateway annotation that is used to
	// disable STUNner's blocking of mixed-protocol LBs for specific Gateways.  If false or any
	// other string other than true, the LB's proto defaults to the first valid listener
	// protocol in the Gateway spec.  If true all valid listener protocols will be added to the
	// LB.
	MixedProtocolAnnotationKey = "stunner.l7mp.io/enable-mixed-protocol-lb"

	// MixedProtocolAnnotationValue is the expected value in order to enable mixed protocol LBs.
	MixedProtocolAnnotationValue = "true"

	// ExternalTrafficPolicyAnnotationKey is the name(key) of the Gateway annotation that is
	// used to control whether ExternalTrafficPolicy=Local is enabled on a LB Service that
	// exposes a Gateway, see https://github.com/l7mp/stunner-gateway-operator/issues/47.
	ExternalTrafficPolicyAnnotationKey = "stunner.l7mp.io/external-traffic-policy"

	// ExternalTrafficPolicyAnnotationValue is the expected value in order to
	// ExternalTrafficPolicy=Local.
	ExternalTrafficPolicyAnnotationValue = "local"

	// ManagedDataplaneDisabledAnnotationKey is the name(key) of the Gateway annotation that is
	// used to prevent the operator from creating a `stunnerd` dataplane Deployment for a
	// Gateway.
	ManagedDataplaneDisabledAnnotationKey = "stunner.l7mp.io/disable-managed-dataplane"

	// ManagedDataplaneDisabledAnnotationValue is the value that can be used to disable the
	// managed dataplane feature for a Gateway.
	ManagedDataplaneDisabledAnnotationValue = "true"

	// NodePortAnnotationKey is the name(key) of the Gateway annotation that is used to select
	// particular nodeports per listener for the LB service, see
	// https://github.com/l7mp/stunner/issues/137.
	NodePortAnnotationKey = "stunner.l7mp.io/nodeport"

	// TargetPortAnnotationKey is the name(key) of the Gateway annotation that is used to select
	// particular targetports per listener for the LB service, see
	// https://github.com/l7mp/stunner/issues/137.
	TargetPortAnnotationKey = "stunner.l7mp.io/targetport"

	// DisableHealthCheckExposeAnnotationKey is the name(key) of the Gateway annotation that is
	// used to disable the LB service to expose the health-check port. Adding the health-check
	// service-port seems to be required by some cloud providers for exposing UDP listeners on
	// LB Services (hence the default), but this annotation allows to disable this on a
	// per-Gayteway basis due to the potential security implications, see
	// https://github.com/l7mp/stunner-gateway-operator/issues/49.
	DisableHealthCheckExposeAnnotationKey = "stunner.l7mp.io/disable-health-check-expose"

	// DisableHealthCheckExposeAnnotationValue is the value that can be used to disable the
	// exposing the health-check port.
	DisableHealthCheckExposeAnnotationValue = "true"

	// DisableSessionAffiffinityAnnotationKey is a Gateway annotation to prevent STUNner from
	// applying the sessionAffinity=client setting in the LB service. Normally this setting
	// improves stability by ensuring that TURN sessions are pinned to the right dataplane
	// pod. However, certain ingress controllers (in particular, Oracle Kubernetes) reject UDP
	// LB services that have this setting on, breaking STUNner installations on these systems,
	// see https://github.com/l7mp/stunner/issues/155. Setting this annotation to "true" for a
	// Gateway will remove this setting from the LB Service created STUNner for the Gateway in
	// order to improve compatibility with Kubernetes deployments that reject it. Default is to
	// apply the session affinity setting.
	DisableSessionAffiffinityAnnotationKey = "stunner.l7mp.io/disable-session-affinity"

	// DisableSessionAffiffinityAnnotationValue is the value that can be used to remove
	// session-affinity settings from the LB service.
	DisableSessionAffiffinityAnnotationValue = "true"

	// EnableRelayAddressDiscoveryAnnotationKey is a Gateway annotation to allow STUNner to
	// discover the public address of the node it is scheduled to run on. By default the relay
	// address in STUNner's TURN listeners is initialized to the value of the $STUNNER_ADDR
	// environment variable, which defaults to the pod IP address. However, pod address is
	// (usually) private even when STUNner is deployed to the host-network namespace, which
	// prevents STUNner from implementing a public TURN server as public TURN servers must
	// return a public IP as the TURN relay address. This feature allows STUNner to set the
	// relay address of STUNner's TURN listeners to the status.ExternalIP (if any) of the node
	// it is running on by .  Default is to disable relay address discovery. Note that as of
	// STUNner v1.1 this feature is available only in the premium tiers.
	EnableRelayAddressDiscoveryAnnotationKey = "stunner.l7mp.io/enable-relay-address-discovery"

	// EnableRelayAddressDiscoveryAnnotationValue is the value that can be used to enable relay
	// address discovery.
	EnableRelayAddressDiscoveryAnnotationValue = "true"

	// DefaultSTUNnerAddressEnvVarName is the environment variable used for configuring
	// stunnerd default listener address.
	DefaultSTUNnerAddressEnvVarName string = "$" + stnrconfv1.DefaultEnvVarAddr

	// NodeAddressPlaceholder is used internally by the operator to let the renderer to signal
	// to the CDS server's config patcher to replace the listener address with the node
	// external IP.
	NodeAddressPlaceholder = "__node_address_placeholder" // guaranteed to not parse as a valid IP
)

var (
	// DefaultHealthCheckEndpoint is the default URI at which health-check requests are served.
	DefaultHealthCheckEndpoint = fmt.Sprintf("http://:%d", stnrconfv1.DefaultHealthCheckPort)

	// DefaultMetricsEndpoint is the default URI at which metrics scaping requests are served.
	DefaultMetricsEndpoint = fmt.Sprintf("http://:%d/metrics", stnrconfv1.DefaultMetricsPort)
)
