package renderer

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

type addrPort struct {
	addr string
	port int
}

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
	v, found := as[opdefault.RelatedGatewayAnnotationKey]
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
	if isOwner(gw, svc, "Gateway") {
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

// taken from redhat operator-utils: https://github.com/redhat-cop/operator-utils/blob/master/pkg/util/owner.go
func isOwner(owner, owned metav1.Object, kind string) bool {
	for _, ownerRef := range owned.GetOwnerReferences() {
		if ownerRef.Name == owner.GetName() && ownerRef.UID == owner.GetUID() &&
			ownerRef.Kind == kind {
			return true
		}
	}

	return false
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
				opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
				opdefault.AppLabelKey:     opdefault.AppLabelValue,
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayAnnotationKey: store.GetObjectKey(gw),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     opdefault.DefaultServiceType,
			Selector: map[string]string{opdefault.AppLabelKey: opdefault.AppLabelValue},
			Ports:    []corev1.ServicePort{},
		},
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

	// copy all listener ports/protocols from the gateway
	// proto defaults to the first valid listener protocol
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
		} else if proto != serviceProto {
			c.log.V(1).Info("createLbService4Gateway: refusing to add listener to service as the listener "+
				"protocol is different from the service protocol (multi-protocol LB services are not supported)",
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

	// Set health check port
	regexProtocol := `^service\.beta\.kubernetes\.io\/.*health.*protocol$`
	regexPort := `^service\.beta\.kubernetes\.io\/.*health.*port$`

	annotationRegexProtocol := regexp.MustCompile(regexProtocol)
	annotationRegexPort := regexp.MustCompile(regexPort)

	var healthCheckPort int32
	healthCheckProtocol := "TCP"

	// Find health check port
	for annotationKey, annotation := range c.gwConf.Spec.LoadBalancerServiceAnnotations {
		match := annotationRegexPort.FindStringSubmatch(annotationKey)
		if len(match) > 0 {
			p, err := strconv.ParseInt(annotation, 10, 32)
			if err != nil {
				c.log.V(1).Error(err, "%s annotation value can't be parsed as an int", match[0])
			} else {
				healthCheckPort = int32(p)
			}
		}
	}

	// Find health check protocol
	for annotationKey, annotation := range c.gwConf.Spec.LoadBalancerServiceAnnotations {
		match := annotationRegexProtocol.FindStringSubmatch(annotationKey)
		if len(match) > 0 {
			switch strings.ToUpper(annotation) {
			case "TCP", "HTTP":
				healthCheckProtocol = "TCP"
			default:
				c.log.V(1).Info("createLbService4Gateway: unknown health check protocol",
					"gateway", store.GetObjectKey(gw), "protocol", string(healthCheckProtocol))
			}
		}
	}

	if healthCheckPort > 0 {
		svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
			Name:     "health-check",
			Protocol: corev1.Protocol(healthCheckProtocol),
			Port:     int32(healthCheckPort),
		})
	}

	// copy the LoadBalancer annotations, if any, from the GatewayConfig to the Service
	for k, v := range c.gwConf.Spec.LoadBalancerServiceAnnotations {
		svc.ObjectMeta.Annotations[k] = v
	}

	// copy the Gateway annotations, if any, to the Service, updating the default from the service
	for k, v := range gw.GetAnnotations() {
		svc.ObjectMeta.Annotations[k] = v
	}

	// forward the first requested address to Kubernetes
	if len(gw.Spec.Addresses) > 0 {
		if gw.Spec.Addresses[0].Type == nil ||
			(gw.Spec.Addresses[0].Type != nil &&
				*gw.Spec.Addresses[0].Type == gwapiv1a2.IPAddressType) {
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
