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

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

type addrPort struct {
	addr string
	port int
}

var annotationRegexProtocol *regexp.Regexp = regexp.MustCompile(`^service\.beta\.kubernetes\.io\/.*health.*protocol$`)
var annotationRegexPort *regexp.Regexp = regexp.MustCompile(`^service\.beta\.kubernetes\.io\/.*health.*port$`)

// returns the preferred address/port exposition for a gateway
// preference order:
// - loadbalancer svc created by us (owned by the gateway)
// - nodeport svc created by us (owned by the gateway)
// - load-balancer svc created manually by a user but annotated for the gateway
// - nodeport svc created manually by a user but annotated for the gateway
func (r *Renderer) getPublicAddrPort4Gateway(gw *gwapiv1a2.Gateway) (*addrPort, error) {
	r.log.V(4).Info("getPublicAddrs4Gateway", "gateway", store.GetObjectKey(gw))
	ownSvcFound := false
	aps := []addrPort{}

	// hint the public address: if the Gateway contains a Spec.Addresses then use that as a
	// fallback for the public address
	addrHint := ""
	if len(gw.Spec.Addresses) > 0 && (gw.Spec.Addresses[0].Type == nil ||
		(gw.Spec.Addresses[0].Type != nil && *gw.Spec.Addresses[0].Type ==
			gwapiv1a2.IPAddressType)) && gw.Spec.Addresses[0].Value != "" {
		addrHint = gw.Spec.Addresses[0].Value
		r.log.V(2).Info("found public address in Gateway.Spec.Addresses, using as a hint",
			"gateway", store.GetObjectKey(gw), "address-hint", addrHint)
	}

	for _, svc := range store.Services.GetAll() {
		r.log.V(4).Info("considering service", "svc", store.GetObjectKey(svc), "status",
			fmt.Sprintf("%#v", svc.Status))

		if r.isServiceAnnotated4Gateway(svc, gw) {
			r.log.V(4).Info("service is annotated for gateway", "svc",
				store.GetObjectKey(svc), "gateway", store.GetObjectKey(svc))

			ap, lb, own := getPublicAddrPort4Svc(svc, gw, addrHint)
			if ap == nil {
				r.log.V(4).Info("service: public address/port not found", "svc",
					store.GetObjectKey(svc), "load-balancer", lb, "own", own)
				continue
			}

			r.log.V(4).Info("service: ready", "svc", store.GetObjectKey(svc),
				"load-balancer", lb, "own", own, "address-port",
				fmt.Sprintf("%s:%d", ap.addr, ap.port))

			if lb {
				// prepend!
				aps = append([]addrPort{*ap}, aps...)
			} else {
				// append
				aps = append(aps, *ap)
			}

			if own {
				r.log.V(4).Info("public service found", "svc",
					store.GetObjectKey(svc), "gateway",
					store.GetObjectKey(gw))

				// we have found the best candidate
				ownSvcFound = true
				break
			}
		}
	}

	var err error
	if !ownSvcFound {
		err = errors.New("load-balancer service not found for gateway: owner-ref missing")
	}

	var ap *addrPort
	apStr := "<NIL>"
	if len(aps) > 0 {
		ap = &aps[0]
		apStr = fmt.Sprintf("%s:%d", ap.addr, ap.port)
	}

	r.log.V(4).Info("getPublicAddrs4Gateway: ready", "gateway", gw.GetName(), "address/port",
		apStr, "own-service-found", ownSvcFound)

	return ap, err
}

// we need the namespaced name!
func (r *Renderer) isServiceAnnotated4Gateway(svc *corev1.Service, gw *gwapiv1a2.Gateway) bool {
	r.log.V(4).Info("isServiceAnnotated4Gateway", "service", store.GetObjectKey(svc),
		"gateway", store.GetObjectKey(gw), "annotations", fmt.Sprintf("%#v",
			svc.GetAnnotations()))

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
func getPublicAddrPort4Svc(svc *corev1.Service, gw *gwapiv1a2.Gateway, addrHint string) (*addrPort, bool, bool) {
	var ap *addrPort

	own := false
	if store.IsOwner(gw, svc, "Gateway") {
		own = true
	}

	i, found := getServicePort(gw, svc)

	// https://github.com/l7mp/stunner-gateway-operator/issues/3
	//
	// Accordingly, the desired selection of public IP should go in the following order:
	//
	// 1. Gateway.Spec.Addresses[0] + Gateway.Spec.Listeners[0].Port
	if found && i < len(svc.Spec.Ports) && addrHint != "" {
		svcPort := svc.Spec.Ports[i]
		ap = &addrPort{
			addr: addrHint,
			port: int(svcPort.Port),
		}
		return ap, false, own
	}

	// 2. If Address is not set, we use the LoadBalancer IP and the above listener port
	if found && svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		lbStatus := svc.Status.LoadBalancer

		// we take the first ingress address assigned by the ingress controller
		if ap := getLBAddrPort4ServicePort(svc, &lbStatus, i); ap != nil {
			return ap, true, own
		}
	}

	// 3. If Address is not set and there is no LoadBalancer IP, we use the first node's IP and
	// NodePort
	if found && i < len(svc.Spec.Ports) {
		svcPort := svc.Spec.Ports[i]
		if svcPort.NodePort > 0 {
			ap = &addrPort{
				addr: getFirstNodeAddr(),
				port: int(svcPort.NodePort),
			}
			return ap, false, own
		}
	}

	return nil, false, own
}

// first matching listener-proto-port and service-proto-port pair
func getServicePort(gw *gwapiv1a2.Gateway, svc *corev1.Service) (int, bool) {
	for _, l := range gw.Spec.Listeners {
		for i, s := range svc.Spec.Ports {
			if int32(l.Port) == s.Port {
				p := ""
				switch l.Protocol {
				case "TCP":
					p = "TCP"
				case "UDP":
					p = "UDP"
				case "TLS":
					p = "TCP"
				case "DTLS":
					p = "UDP"
				}

				if strings.EqualFold(p, string(s.Protocol)) {
					return i, true
				}
			}
		}
	}
	return 0, false
}

// first matching service-port and load-balancer service status
func getLBAddrPort4ServicePort(svc *corev1.Service, st *corev1.LoadBalancerStatus, spIndex int) *addrPort {
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

			// fallback to Hostname (typically for AWS)
			a := s.IP
			if a == "" {
				a = s.Hostname
			}

			return &addrPort{
				addr: a,
				port: int(s.Ports[spIndex].Port),
			}
		}
	}

	// some load-balancer controllers do not include a status.Ingress[x].Ports substatus: we
	// fall back to the first load-balancer IP we find and use the port from the service-port
	// as a port
	if len(st.Ingress) > 0 {
		// fallback to Hostname (typically for AWS)
		a := st.Ingress[0].IP
		if a == "" {
			a = st.Ingress[0].Hostname
		}

		return &addrPort{
			addr: a,
			port: int(port),
		}
	}

	return nil
}

// we always take the FIRST listener port in the gateway: if you want to expose STUNner on multiple
// ports, use separate Gateways!
func createLbService4Gateway(c *RenderContext, gw *gwapiv1a2.Gateway) *corev1.Service {
	if len(gw.Spec.Listeners) == 0 {
		// should never happen
		return nil
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: gw.GetNamespace(),
			Name:      gw.GetName(),
			Labels: map[string]string{
				opdefault.OwnedByLabelKey:         opdefault.OwnedByLabelValue,
				opdefault.RelatedGatewayNamespace: gw.GetNamespace(),
				opdefault.RelatedGatewayKey:       gw.GetName(),
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayKey: store.GetObjectKey(gw),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     opdefault.DefaultServiceType,
			Selector: map[string]string{},
			Ports:    []corev1.ServicePort{},
		},
	}

	// set labels
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

	// merge both GatewayConfig.spec.LBServiceAnnotations map and
	// Gateway.metadata.Annotations map into a common map
	// this way the MixedProtocolAnnotation can be placed in either of them
	// Annotations from the Gateway will override annotations from the GwConfig
	// if present with the same key
	as := mergeMaps(c.gwConf.Spec.LoadBalancerServiceAnnotations, gw.Annotations)

	isMixedProtocolEnabled, found := as[opdefault.MixedProtocolAnnotationKey]
	// copy all listener ports/protocols from the gateway
	serviceProto := ""
	for _, l := range gw.Spec.Listeners {
		var proto string
		switch string(l.Protocol) {
		case "UDP", "DTLS":
			proto = "UDP"
		case "TCP", "TLS":
			proto = "TCP"
		default:
			c.log.V(1).Info("createLbService4Gateway: unknown listener protocol",
				"gateway", store.GetObjectKey(gw), "listener", l.Name, "protocol", string(l.Protocol))
			continue
		}
		if serviceProto == "" {
			serviceProto = proto
		} else if found && isMixedProtocolEnabled == opdefault.MixedProtocolAnnotationValue {
			serviceProto = proto
		} else if proto != serviceProto {
			c.log.V(1).Info("createLbService4Gateway: refusing to add listener to service as the listener "+
				"protocol is different from the service protocol (multi-protocol LB services are disabled by default)",
				"gateway", store.GetObjectKey(gw), "listener", l.Name, "listener-protocol", proto,
				"service-protocol", serviceProto)
			continue
		}
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:     fmt.Sprintf("%s-%s", gw.GetName(), strings.ToLower(serviceProto)),
			Protocol: corev1.Protocol(serviceProto),
			Port:     int32(l.Port),
		})
	}

	healthCheckPort, err := setHealthCheck(as, svc)
	if err != nil {
		c.log.V(1).Info("could not set health check port", "error", err.Error())
	} else if healthCheckPort != 0 {
		c.log.V(1).Info("health check port opened", "port", healthCheckPort)
	}

	// copy the LoadBalancer annotations from the GatewayConfig
	// and the Gateway Annotations to the Service
	for k, v := range as {
		svc.ObjectMeta.Annotations[k] = v
	}

	// forward the first requested address to Kubernetes
	if len(gw.Spec.Addresses) > 0 {
		if gw.Spec.Addresses[0].Type == nil ||
			(gw.Spec.Addresses[0].Type != nil &&
				*gw.Spec.Addresses[0].Type == gwapiv1a2.IPAddressType) {
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

	return svc
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

	if healthCheckPort > 0 && healthCheckProtocol != "" {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:     "gateway-health-check",
			Protocol: corev1.Protocol(healthCheckProtocol),
			Port:     int32(healthCheckPort),
		})
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
