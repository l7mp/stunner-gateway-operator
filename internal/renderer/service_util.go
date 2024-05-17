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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

var annotationRegexProtocol *regexp.Regexp = regexp.MustCompile(`^service\.beta\.kubernetes\.io\/.*health.*protocol$`)
var annotationRegexPort *regexp.Regexp = regexp.MustCompile(`^service\.beta\.kubernetes\.io\/.*health.*port$`)

type gwAddrPort struct {
	aType gwapiv1.AddressType
	addr  string
	port  int
}

func (ap gwAddrPort) isEmpty() bool {
	if ap.addr == "" || ap.port == 0 {
		return true
	}
	return false
}

func (ap gwAddrPort) String() string {
	if ap.isEmpty() {
		return "<nil>"
	}
	return fmt.Sprintf("%s(type:%s):%d", ap.addr, string(ap.aType), ap.port)
}

// returns the preferred address/port exposition for all listeners of the gateway
// preference order: loadbalancer svc > nodeport svc
func (r *Renderer) getPublicAddr(gw *gwapiv1.Gateway) ([]gwAddrPort, error) {
	aps := make([]gwAddrPort, len(gw.Spec.Listeners))

	// find our service
	svc, err := r.getPublicSvc(gw)
	if err != nil {
		return aps, err
	}

	// find the addr-port per each listener
	status := make([]string, len(gw.Spec.Listeners))
	var retErr error
	for i, l := range gw.Spec.Listeners {
		status[i] = "<nil>"
		ap, err := r.getPublicListenerAddr(svc, gw, &gw.Spec.Listeners[i])
		if err != nil {
			r.log.Info("could not find public adddress for listener",
				"gateway", store.GetObjectKey(gw), "listener", l.Name,
				"error", err.Error())
			retErr = NewNonCriticalError(PublicListenerAddressNotFound)
			continue
		}
		aps[i] = ap
		status[i] = ap.String()
	}

	r.log.V(4).Info("searching public address for gateway: ready",
		"gateway", store.GetObjectKey(gw),
		"address", strings.Join(status, ","))

	return aps, retErr
}

func (r *Renderer) getPublicSvc(gw *gwapiv1.Gateway) (*corev1.Service, error) {
	var pubSvc *corev1.Service
	for _, svc := range store.Services.GetAll() {
		if !isServiceAnnotated4Gateway(svc, gw) {
			r.log.V(4).Info("skipping service: not annotated for gateway", "svc",
				store.GetObjectKey(svc), "gateway", store.GetObjectKey(svc))
			continue
		}

		if !store.IsOwner(gw, svc, "Gateway") {
			r.log.V(4).Info("skipping service: no owner-reference to gateway", "svc",
				store.GetObjectKey(svc), "gateway", store.GetObjectKey(svc))
			continue
		}

		if isSvcPreferred(pubSvc, svc) {
			r.log.V(4).Info("found service", "svc", store.GetObjectKey(svc))
			pubSvc = svc
		}
	}

	if pubSvc == nil {
		return nil, NewNonCriticalError(PublicAddressNotFound)
	}

	return pubSvc, nil
}

func isServiceAnnotated4Gateway(svc *corev1.Service, gw *gwapiv1.Gateway) bool {
	as := svc.GetAnnotations()
	namespacedName := fmt.Sprintf("%s/%s", gw.GetNamespace(), gw.GetName())
	v, found := as[opdefault.RelatedGatewayKey]
	if found && v == namespacedName {
		return true
	}

	return false
}

// precedence: ClusterIP < NodePort < ExternalName < LB
func isSvcPreferred(a, b *corev1.Service) bool {
	if a == nil {
		return true
	}

	switch a.Spec.Type {
	case "ClusterIP":
		return b.Spec.Type == corev1.ServiceTypeNodePort ||
			b.Spec.Type == corev1.ServiceTypeExternalName ||
			b.Spec.Type == corev1.ServiceTypeLoadBalancer
	case "NodePort":
		return b.Spec.Type == corev1.ServiceTypeExternalName ||
			b.Spec.Type == corev1.ServiceTypeLoadBalancer
	case "ExternalName":
		return b.Spec.Type == corev1.ServiceTypeLoadBalancer
	case "LoadBalancer":
		return false
	}

	return false
}

func (r *Renderer) getPublicListenerAddr(svc *corev1.Service, gw *gwapiv1.Gateway, l *gwapiv1.Listener) (gwAddrPort, error) {
	serviceProto, err := r.getServiceProtocol(l.Protocol)
	if err != nil {
		return gwAddrPort{}, err
	}

	// find the right service-port
	var sp *corev1.ServicePort
	var spIndex int
	for i, s := range svc.Spec.Ports {
		if int32(l.Port) == s.Port && strings.EqualFold(serviceProto, string(s.Protocol)) {
			sp = &svc.Spec.Ports[i]
			spIndex = i
			break
		}
	}

	if sp == nil {
		return gwAddrPort{}, errors.New("cannot find matching service-port for listener" +
			"(hint: enable mixed-protocol-LB support)")
	}

	// Public IPs weighed in the following order: (see
	// https://github.com/l7mp/stunner-gateway-operator/issues/3)
	//
	// 1. Gateway.Spec.Addresses[0] + Gateway.Spec.Listeners[0].Port
	if len(gw.Spec.Addresses) > 0 && gw.Spec.Addresses[0].Value != "" {
		t := gwapiv1.IPAddressType
		if gw.Spec.Addresses[0].Type != nil {
			t = *gw.Spec.Addresses[0].Type
		}
		ap := gwAddrPort{
			aType: t,
			addr:  gw.Spec.Addresses[0].Value,
			port:  int(sp.Port),
		}

		r.log.V(4).Info("using requested address from Gateway spec for listener",
			"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw),
			"listener", l.Name, "address", ap.String())

		return ap, nil
	}

	// 2. If Address is not set, we use the LoadBalancer IP and the above listener port
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if ap := getLBAddr(svc, spIndex); ap != nil {
			r.log.V(4).Info("using LoadBalancer address for listener",
				"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw),
				"listener", l.Name, "address", ap.String())
			return *ap, nil
		}
	}

	// 3. If Address is not set and there is no LoadBalancer IP, we use the first node's IP and
	// NodePort
	if addr := getFirstNodeAddr(); addr != "" && sp.NodePort > 0 {
		ap := gwAddrPort{
			aType: gwapiv1.IPAddressType,
			addr:  addr,
			port:  int(sp.NodePort),
		}

		r.log.V(4).Info("using NodePort address for listener",
			"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw),
			"listener", l.Name, "address", ap.String())

		return ap, nil
	}

	return gwAddrPort{}, errors.New("could not usable public address for listener")
}

// first matching service-port and load-balancer service status
func getLBAddr(svc *corev1.Service, spIndex int) *gwAddrPort {
	for _, ingressStatus := range svc.Status.LoadBalancer.Ingress {
		// if status contains per-service-port status
		if ingressStatus.Ports != nil && len(ingressStatus.Ports) != 0 && spIndex < len(ingressStatus.Ports) {
			// find the status for our service-port
			spStatus := ingressStatus.Ports[spIndex]
			if spStatus.Port != svc.Spec.Ports[spIndex].Port ||
				spStatus.Protocol != svc.Spec.Ports[spIndex].Protocol {
				continue
			}

			// if IP address is available, use it
			if ingressStatus.IP != "" {
				return &gwAddrPort{
					aType: gwapiv1.IPAddressType,
					addr:  ingressStatus.IP,
					port:  int(spStatus.Port),
				}
			}

			// fallback to Hostname (typically for AWS)
			if ingressStatus.Hostname != "" {
				return &gwAddrPort{
					aType: gwapiv1.HostnameAddressType,
					addr:  ingressStatus.Hostname,
					port:  int(spStatus.Port),
				}
			}
		}
	}

	// some load-balancer controllers do not include a status.Ingress[x].Ports substatus: we
	// fall back to the first load-balancer IP we find and use the port from the service-port
	// as a port
	if len(svc.Status.LoadBalancer.Ingress) > 0 {
		ingressStatus := svc.Status.LoadBalancer.Ingress[0]

		// if IP address is available, use it
		if ingressStatus.IP != "" {
			return &gwAddrPort{
				aType: gwapiv1.IPAddressType,
				addr:  ingressStatus.IP,
				port:  int(svc.Spec.Ports[spIndex].Port),
			}
		}

		// fallback to Hostname (typically for AWS)
		if ingressStatus.Hostname != "" {
			return &gwAddrPort{
				aType: gwapiv1.HostnameAddressType,
				addr:  ingressStatus.Hostname,
				port:  int(svc.Spec.Ports[spIndex].Port),
			}
		}
	}

	return nil
}

func (r *Renderer) createLbService4Gateway(c *RenderContext, gw *gwapiv1.Gateway) *corev1.Service {
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
		svc.Spec.Type = opdefault.DefaultServiceType
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
		for i := range svc.Spec.Ports {
			s := &svc.Spec.Ports[i]
			if string(l.Name) == s.Name {
				// found one, let's update it and move on
				s.Protocol = corev1.Protocol(serviceProto)
				s.Port = int32(l.Port)
				s.TargetPort = intstr.FromInt(int(l.Port))
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
				*gw.Spec.Addresses[0].Type == gwapiv1.IPAddressType) {
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

// getServiceProtocol returns the sercice-compatible protocol for a listener
func (r *Renderer) getServiceProtocol(proto gwapiv1.ProtocolType) (string, error) {
	protocol, err := r.getProtocol(proto)
	if err != nil {
		return "", err
	}

	var serviceProto string
	switch protocol {
	case stnrconfv1.ListenerProtocolUDP, stnrconfv1.ListenerProtocolDTLS:
		serviceProto = "UDP"
	case stnrconfv1.ListenerProtocolTURNUDP, stnrconfv1.ListenerProtocolTURNDTLS:
		serviceProto = "UDP"
	case stnrconfv1.ListenerProtocolTURNTCP, stnrconfv1.ListenerProtocolTURNTLS:
		serviceProto = "TCP"
	case stnrconfv1.ListenerProtocolTCP, stnrconfv1.ListenerProtocolTLS:
		serviceProto = "TCP"
	default:
		return "", NewNonCriticalError(InvalidProtocol)
	}

	return serviceProto, nil
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
