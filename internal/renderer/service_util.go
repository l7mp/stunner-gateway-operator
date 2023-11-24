package renderer

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

type gatewayAddress struct {
	aType gwapiv1b1.AddressType
	addr  string
	port  int
}

func (ap *gatewayAddress) String() string {
	if ap == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s:%d(type:%s)", ap.addr, ap.port, string(ap.aType))
}

var annotationRegexProtocol *regexp.Regexp = regexp.MustCompile(`^service\.beta\.kubernetes\.io\/.*health.*protocol$`)
var annotationRegexPort *regexp.Regexp = regexp.MustCompile(`^service\.beta\.kubernetes\.io\/.*health.*port$`)

// returns the preferred address/port exposition for a gateway
// preference order:
// - loadbalancer svc created by us (owned by the gateway)
// - nodeport svc created by us (owned by the gateway)
// - load-balancer svc created manually by a user but annotated for the gateway
// - nodeport svc created manually by a user but annotated for the gateway
func (r *Renderer) getPublicAddrPort4Gateway(gw *gwapiv1b1.Gateway) (*gatewayAddress, error) {
	r.log.V(4).Info("getPublicAddrs4Gateway", "gateway", store.GetObjectKey(gw))
	aps := []gatewayAddress{}

	// hint the public address: if the Gateway contains a Spec.Addresses then use that as a
	// fallback for the public address
	var addrHint gatewayAddress
	if len(gw.Spec.Addresses) > 0 && (gw.Spec.Addresses[0].Type == nil ||
		*gw.Spec.Addresses[0].Type == gwapiv1b1.IPAddressType || *gw.Spec.Addresses[0].Type == gwapiv1b1.HostnameAddressType) &&
		gw.Spec.Addresses[0].Value != "" {
		addrHint = gatewayAddress{
			aType: *gw.Spec.Addresses[0].Type,
			addr:  gw.Spec.Addresses[0].Value,
		}
		r.log.V(2).Info("found public address in Gateway.Spec.Addresses",
			"gateway", store.GetObjectKey(gw), "address", addrHint.String())
	}

	err := NewNonCriticalError(PublicAddressNotFound)
	for _, svc := range store.Services.GetAll() {
		r.log.V(4).Info("considering service", "svc", store.GetObjectKey(svc), "status",
			fmt.Sprintf("%#v", svc.Status))

		if !r.isServiceAnnotated4Gateway(svc, gw) {
			r.log.V(4).Info("skipping service: not annotated for gateway", "svc",
				store.GetObjectKey(svc), "gateway", store.GetObjectKey(svc))
			continue
		}

		if !store.IsOwner(gw, svc, "Gateway") {
			r.log.V(4).Info("skipping service: no owner-reference to gateway", "svc",
				store.GetObjectKey(svc), "gateway", store.GetObjectKey(svc))
			continue
		}

		ap, lb := r.getPublicAddrPort4Svc(svc, gw, addrHint)
		if ap == nil {
			r.log.V(4).Info("public address/port not found for service", "svc",
				store.GetObjectKey(svc), "gateway", store.GetObjectKey(svc))
			continue
		}

		r.log.V(4).Info("public address/port found for service", "svc",
			store.GetObjectKey(svc), "address", ap.String(), "load-balancer", lb)

		if lb {
			// prepend!
			aps = append([]gatewayAddress{*ap}, aps...)
		} else {
			// append
			aps = append(aps, *ap)
		}

		err = nil
	}

	var ap *gatewayAddress
	if len(aps) > 0 {
		ap = &aps[0]
	}

	r.log.V(4).Info("getPublicAddrs4Gateway: ready", "gateway", gw.GetName(), "address", ap.String())

	return ap, err
}

// we need the namespaced name!
func (r *Renderer) isServiceAnnotated4Gateway(svc *corev1.Service, gw *gwapiv1b1.Gateway) bool {
	// r.log.V(4).Info("isServiceAnnotated4Gateway", "service", store.GetObjectKey(svc),
	// 	"gateway", store.GetObjectKey(gw), "annotations", fmt.Sprintf("%#v",
	// 		svc.GetAnnotations()))

	as := svc.GetAnnotations()
	namespacedName := fmt.Sprintf("%s/%s", gw.GetNamespace(), gw.GetName())
	v, found := as[opdefault.RelatedGatewayKey]
	if found && v == namespacedName {
		// r.log.V(4).Info("isServiceAnnotated4Gateway: service annotated for gateway",
		// 	"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw))
		return true
	}

	return false
}

// for the semantics, see https://github.com/l7mp/stunner-gateway-operator/issues/3
func (r *Renderer) getPublicAddrPort4Svc(svc *corev1.Service, gw *gwapiv1b1.Gateway, addrHint gatewayAddress) (*gatewayAddress, bool) {
	var ap *gatewayAddress

	i, found := r.getServicePort(gw, svc)

	// The desired selection of public IP should go in the following order: (see
	// https://github.com/l7mp/stunner-gateway-operator/issues/3)
	//
	// 1. Gateway.Spec.Addresses[0] + Gateway.Spec.Listeners[0].Port
	if found && i < len(svc.Spec.Ports) && addrHint.addr != "" {
		svcPort := svc.Spec.Ports[i]
		ap = &gatewayAddress{
			aType: addrHint.aType,
			addr:  addrHint.addr,
			port:  int(svcPort.Port),
		}
		r.log.V(4).Info("getPublicAddrPort4Svc: using requested address from Gateway spec",
			"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw), "address", ap.String())
		return ap, false
	}

	// 2. If Address is not set, we use the LoadBalancer IP and the above listener port
	if found && svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		lbStatus := svc.Status.LoadBalancer

		// we take the first ingress address assigned by the ingress controller
		if ap := getLBAddrPort4ServicePort(svc, &lbStatus, i); ap != nil {
			r.log.V(4).Info("getPublicAddrPort4Svc: using LoadBalancer address",
				"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw),
				"address", ap.String())
			return ap, true
		}
	}

	// 3. If Address is not set and there is no LoadBalancer IP, we use the first node's IP and
	// NodePort
	if found && i < len(svc.Spec.Ports) {
		svcPort := svc.Spec.Ports[i]
		addr := getFirstNodeAddr()
		if svcPort.NodePort > 0 && addr != "" {
			ap = &gatewayAddress{
				aType: gwapiv1b1.IPAddressType,
				addr:  addr,
				port:  int(svcPort.NodePort),
			}
			r.log.V(4).Info("getPublicAddrPort4Svc: using NodePort address",
				"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw),
				"address", ap.String())
			return ap, false
		}
	}

	return nil, false
}

func (r *Renderer) createLbService4Gateway(c *RenderContext, gw *gwapiv1b1.Gateway) *corev1.Service {
	if len(gw.Spec.Listeners) == 0 {
		// should never happen
		return nil
	}

	// mandatory labels and annotations
	mandatoryLabels := map[string]string{
		opdefault.OwnedByLabelKey:         opdefault.OwnedByLabelValue,
		opdefault.RelatedGatewayNamespace: gw.GetNamespace(),
		opdefault.RelatedGatewayKey:       gw.GetName(),
	}
	mandatoryAnnotations := mergeMaps(
		// base: GatewayConfig.Spec.LBServiceAnnotations
		c.gwConf.Spec.LoadBalancerServiceAnnotations,
		// Gateway annotations override base
		gw.Annotations,
		// related gateway is always included!
		map[string]string{
			opdefault.RelatedGatewayKey: store.GetObjectKey(gw),
		})

	// Fetch the service as it exists in the store, this should prevent changing fields we shouldn't
	svc := store.Services.GetObject(types.NamespacedName{Namespace: gw.GetNamespace(), Name: gw.GetName()})
	if svc == nil {
		svc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   gw.GetNamespace(),
				Name:        gw.GetName(),
				Labels:      mandatoryLabels,
				Annotations: mandatoryAnnotations,
			},
			Spec: corev1.ServiceSpec{
				Type:     opdefault.DefaultServiceType,
				Selector: map[string]string{},
				Ports:    []corev1.ServicePort{},
			},
		}
	} else {
		// mandatory labels and annotations must always be there
		svc.SetLabels(mergeMaps(svc.GetLabels(), mandatoryLabels))
		svc.SetAnnotations(mergeMaps(svc.GetAnnotations(), mandatoryAnnotations))
	}

	// set selectors
	switch config.DataplaneMode {
	case config.DataplaneModeLegacy:
		// legacy mode: note that this may break for multiple gateway hierarchies but we
		// leave it as is for compatibility
		svc.Spec.Selector = map[string]string{
			opdefault.AppLabelKey: opdefault.AppLabelValue,
		}
	case config.DataplaneModeManaged:
		svc.Spec.Selector = map[string]string{
			opdefault.AppLabelKey:             opdefault.AppLabelValue,
			opdefault.RelatedGatewayNamespace: gw.GetNamespace(),
			opdefault.RelatedGatewayKey:       gw.GetName(),
		}
	}

	// update service type if necessary
	svcType := string(opdefault.DefaultServiceType)
	if t, ok := c.gwConf.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey]; ok {
		svcType = t
	}
	if t, ok := gw.GetAnnotations()[opdefault.ServiceTypeAnnotationKey]; ok {
		svcType = t
	}

	switch svcType {
	case "ClusterIP":
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	case "NodePort":
		svc.Spec.Type = corev1.ServiceTypeNodePort
	case "ExternalName":
		svc.Spec.Type = corev1.ServiceTypeExternalName
	case "LoadBalancer":
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	default:
		svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	}

	mixedProto := false
	if isMixedProtocolEnabled, found := svc.GetAnnotations()[opdefault.MixedProtocolAnnotationKey]; found {
		mixedProto = isMixedProtocolEnabled == opdefault.MixedProtocolAnnotationValue
	}
	// copy all listener ports/protocols from the gateway
	serviceProto := ""
	for _, l := range gw.Spec.Listeners {
		var proto string

		proto, err := r.getServiceProtocol(l.Protocol)
		if err != nil {
			continue
		}

		// set service-port.protocol to the listener protocol
		if serviceProto == "" || mixedProto {
			serviceProto = proto
		}

		// warn if gateway uses multiple listener protocols but mixedProto is not set
		if proto != serviceProto {
			c.log.V(1).Info("createLbService4Gateway: refusing to add listener to service as the listener "+
				"protocol is different from the service protocol (multi-protocol LB services are disabled by default)",
				"gateway", store.GetObjectKey(gw), "listener", l.Name, "listener-protocol", proto,
				"service-protocol", serviceProto)
			continue
		}

		servicePortExists := false
		// search for existing port
		for _, s := range svc.Spec.Ports {
			if string(l.Name) == s.Name {
				// found one, let's update it and move on
				s.Protocol = corev1.Protocol(serviceProto)
				s.Port = int32(l.Port)
				servicePortExists = true
				break
			}
		}

		if !servicePortExists {
			svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
				Name:     string(l.Name),
				Protocol: corev1.Protocol(serviceProto),
				Port:     int32(l.Port),
			})
		}
	}

	// Open the health-check port for LoadBalancer Services only
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		healthCheckPort, err := setHealthCheck(svc.GetAnnotations(), svc)
		if err != nil {
			c.log.V(1).Info("could not set health check port", "error", err.Error())
		} else if healthCheckPort != 0 {
			c.log.V(1).Info("health check port opened", "port", healthCheckPort)
		}
	}

	// forward the first requested address to Kubernetes
	if len(gw.Spec.Addresses) > 0 {
		if gw.Spec.Addresses[0].Type == nil ||
			(gw.Spec.Addresses[0].Type != nil &&
				*gw.Spec.Addresses[0].Type == gwapiv1b1.IPAddressType) {
			// only the first address can be used because
			// stunner is limited to use a single public address
			// https://github.com/l7mp/stunner-gateway-operator/issues/32#issuecomment-1648035135
			svc.Spec.ExternalIPs = []string{gw.Spec.Addresses[0].Value}
			svc.Spec.LoadBalancerIP = gw.Spec.Addresses[0].Value
		}
	}

	// no valid listener in gateway: refuse to create a service
	if len(svc.Spec.Ports) == 0 {
		c.log.V(1).Info("createLbService4Gateway: refusing to create a LB service as there "+
			"is no valid listener found", "gateway", store.GetObjectKey(gw))
		return nil
	}

	if err := controllerutil.SetOwnerReference(gw, svc, r.scheme); err != nil {
		r.log.Error(err, "cannot set owner reference", "owner",
			store.GetObjectKey(gw), "reference",
			store.GetObjectKey(svc))
	}

	return svc
}

// first matching listener-proto-port and service-proto-port pair
func (r *Renderer) getServicePort(gw *gwapiv1b1.Gateway, svc *corev1.Service) (int, bool) {
	for _, l := range gw.Spec.Listeners {
		serviceProto, err := r.getServiceProtocol(l.Protocol)
		if err != nil {
			continue
		}

		for i, s := range svc.Spec.Ports {
			if int32(l.Port) == s.Port {
				if strings.EqualFold(serviceProto, string(s.Protocol)) {
					return i, true
				}
			}
		}
	}
	return 0, false
}

// getServiceProtocol returns the sercice-compatible protocol for a listener
func (r *Renderer) getServiceProtocol(proto gwapiv1b1.ProtocolType) (string, error) {
	protocol, err := r.getProtocol(proto)
	if err != nil {
		return "", err
	}

	var serviceProto string
	switch protocol {
	case stnrconfv1a1.ListenerProtocolUDP, stnrconfv1a1.ListenerProtocolDTLS:
		serviceProto = "UDP"
	case stnrconfv1a1.ListenerProtocolTURNUDP, stnrconfv1a1.ListenerProtocolTURNDTLS:
		serviceProto = "UDP"
	case stnrconfv1a1.ListenerProtocolTURNTCP, stnrconfv1a1.ListenerProtocolTURNTLS:
		serviceProto = "TCP"
	case stnrconfv1a1.ListenerProtocolTCP, stnrconfv1a1.ListenerProtocolTLS:
		serviceProto = "TCP"
	default:
		return "", NewNonCriticalError(InvalidProtocol)
	}

	return serviceProto, nil
}

// first matching service-port and load-balancer service status
func getLBAddrPort4ServicePort(svc *corev1.Service, st *corev1.LoadBalancerStatus, spIndex int) *gatewayAddress {
	// spIndex must point to a valid service-port
	if len(svc.Spec.Ports) == 0 || spIndex >= len(svc.Spec.Ports) {
		fmt.Printf("getLBAddrPort4ServicePort: INTERNAL ERROR: invalid service-port index\n")
		return nil
	}

	proto := svc.Spec.Ports[spIndex].Protocol
	port := svc.Spec.Ports[spIndex].Port

	for _, s := range st.Ingress {
		// index i is valid, and the protocol and port match the ones specified for the gateway
		if len(s.Ports) > 0 && spIndex < len(s.Ports) &&
			s.Ports[spIndex].Port == port && s.Ports[spIndex].Protocol == proto {

			ap := gatewayAddress{
				aType: gwapiv1b1.IPAddressType,
				addr:  s.IP,
				port:  int(s.Ports[spIndex].Port),
			}
			// fallback to Hostname (typically for AWS)
			if s.IP == "" {
				ap.aType = gwapiv1b1.HostnameAddressType
				ap.addr = s.Hostname
			}

			return &ap
		}
	}

	// some load-balancer controllers do not include a status.Ingress[x].Ports substatus: we
	// fall back to the first load-balancer IP we find and use the port from the service-port
	// as a port
	if len(st.Ingress) > 0 {
		ap := gatewayAddress{
			aType: gwapiv1b1.IPAddressType,
			addr:  st.Ingress[0].IP,
			port:  int(port),
		}
		// fallback to Hostname (typically for AWS)
		if ap.addr == "" {
			ap.aType = gwapiv1b1.HostnameAddressType
			ap.addr = st.Ingress[0].Hostname
		}
		return &ap
	}

	return nil
}

func setHealthCheck(annotations map[string]string, svc *corev1.Service) (int32, error) {
	var healthCheckPort int32
	var healthCheckProtocol string

	// Find health check port
	for annotationKey, annotation := range annotations {
		if annotationRegexPort.MatchString(annotationKey) {
			p, err := strconv.ParseInt(annotation, 10, 32)
			if err != nil {
				return 0, err
			} else {
				healthCheckPort = int32(p)
			}
		}
	}

	// Find health check protocol
	for annotationKey, annotation := range annotations {
		if annotationRegexProtocol.MatchString(annotationKey) {
			switch strings.ToUpper(annotation) {
			case "TCP", "HTTP":
				healthCheckProtocol = "TCP"
			default:
				return 0, errors.New("unknown health check protocol")
			}
		}
	}

	healthCheckName := "gateway-health-check"
	if healthCheckPort > 0 && healthCheckProtocol != "" {
		servicePortExists := false
		for _, s := range svc.Spec.Ports {
			if string(s.Name) == healthCheckName {
				// found one, let's update it and move on
				s.Protocol = corev1.Protocol(healthCheckProtocol)
				s.Port = int32(healthCheckPort)
				servicePortExists = true
				break
			}
		}
		if !servicePortExists {
			svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
				Name:     healthCheckName,
				Protocol: corev1.Protocol(healthCheckProtocol),
				Port:     int32(healthCheckPort),
			})
		}
		return int32(healthCheckPort), nil
	}
	return 0, nil
}

func mergeMaps(maps ...map[string]string) map[string]string {
	mergedMap := make(map[string]string)

	for _, m := range maps {
		for k, v := range m {
			mergedMap[k] = v
		}
	}

	return mergedMap
}

// find the ClusterIP associated with a service
func getClusterIP(n types.NamespacedName) ([]string, error) {
	ret := []string{}

	s := store.Services.GetObject(n)
	if s == nil {
		return ret, NewNonCriticalError(ServiceNotFound)
	}

	if s.Spec.ClusterIP == "" || s.Spec.ClusterIP == "None" {
		return ret, NewNonCriticalError(ClusterIPNotFound)
	}

	ret = append(ret, s.Spec.ClusterIP)

	return ret, nil
}
