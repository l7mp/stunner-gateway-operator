package renderer

import (
	"errors"
	"fmt"
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

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
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
func (r *Renderer) getPublicAddrPort4Gateway(gw *gatewayv1alpha2.Gateway) (*addrPort, error) {
	r.log.V(4).Info("getPublicAddrs4Gateway", "gateway", store.GetObjectKey(gw))
	ownSvcFound := false
	aps := []addrPort{}

	for _, svc := range store.Services.GetAll() {
		r.log.V(4).Info("considering service", "svc", store.GetObjectKey(svc), "status",
			fmt.Sprintf("%#v", svc.Status))

		if r.isServiceAnnotated4Gateway(svc, gw) {
			r.log.V(4).Info("service is annotated for gateway", "svc",
				store.GetObjectKey(svc), "gateway", store.GetObjectKey(svc))

			ap, lb, own := getPublicAddrPort4Svc(svc, gw)
			if ap == nil {
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
func (r *Renderer) isServiceAnnotated4Gateway(svc *corev1.Service, gw *gatewayv1alpha2.Gateway) bool {
	r.log.V(4).Info("isServiceAnnotated4Gateway", "service", store.GetObjectKey(svc),
		"gateway", store.GetObjectKey(gw), "annotations", fmt.Sprintf("%#v",
			svc.GetAnnotations()))

	as := svc.GetAnnotations()
	namespacedName := fmt.Sprintf("%s/%s", gw.GetNamespace(), gw.GetName())
	v, found := as[config.GatewayAddressAnnotationKey]
	if found && v == namespacedName {
		// r.log.V(4).Info("isServiceAnnotated4Gateway: service annotated for gateway",
		// 	"service", store.GetObjectKey(svc), "gateway", store.GetObjectKey(gw))
		return true
	}

	return false
}

// FIXME: we assume there's only a single ServicePort in the service ad we use the first one
// available. this is true for the services we create bu may break if user creates funky services
// for us
func getPublicAddrPort4Svc(svc *corev1.Service, gw *gatewayv1alpha2.Gateway) (*addrPort, bool, bool) {
	var ap *addrPort

	own := false
	if isOwner(gw, svc, "Gateway") {
		own = true
	}

	i, found := getServicePort(gw, svc)
	if found && svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		lbStatus := svc.Status.LoadBalancer

		// we take the first ingress address assigned by the ingress controller
		if ap := getLBAddrPort4ServicePort(svc, &lbStatus, i); ap != nil {
			return ap, true, own
		}
	}

	// fall-back to nodeport
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
func getServicePort(gw *gatewayv1alpha2.Gateway, svc *corev1.Service) (int, bool) {
	for _, l := range gw.Spec.Listeners {
		for i, s := range svc.Spec.Ports {
			if strings.EqualFold(string(l.Protocol), string(s.Protocol)) &&
				int32(l.Port) == s.Port {
				return i, true
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
func createLbService4Gateway(c *RenderContext, gw *gatewayv1alpha2.Gateway) *corev1.Service {
	if len(gw.Spec.Listeners) == 0 {
		// should never happen
		return nil
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: gw.GetNamespace(),
			Name:      fmt.Sprintf("stunner-gateway-%s-svc", gw.GetName()),
			Annotations: map[string]string{
				config.GatewayAddressAnnotationKey: store.GetObjectKey(gw),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{config.DefaultStunnerDeploymentLabel: config.DefaultStunnerDeploymentValue},
			Ports: []corev1.ServicePort{
				{
					Name:     fmt.Sprintf("stunner-gateway-%s-udp-port", gw.GetName()),
					Protocol: corev1.Protocol(gw.Spec.Listeners[0].Protocol),
					Port:     int32(gw.Spec.Listeners[0].Port),
				},
			},
		},
	}

	// copy the LoadBalancer annotations, if any, from the GatewayConfig to the Service
	for k, v := range c.gwConf.Spec.LoadBalancerServiceAnnotations {
		svc.ObjectMeta.Annotations[k] = v
	}

	// forward the first requested address to Kubernetes
	if len(gw.Spec.Addresses) > 0 {
		if gw.Spec.Addresses[0].Type == nil ||
			(gw.Spec.Addresses[0].Type != nil && *gw.Spec.Addresses[0].Type == gatewayv1alpha2.IPAddressType) {
			svc.Spec.LoadBalancerIP = gw.Spec.Addresses[0].Value
		}
	}

	return svc
}

// find the ClusterIP associated with a service
func getClusterIP(n types.NamespacedName) []string {
	ret := []string{}

	s := store.Services.GetObject(n)
	if s == nil || s.Spec.ClusterIP == "" || s.Spec.ClusterIP == "None" {
		return ret
	}

	ret = append(ret, s.Spec.ClusterIP)

	return ret
}
