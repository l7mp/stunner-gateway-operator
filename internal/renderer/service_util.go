package renderer

import (
	"fmt"
	"strings"

	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) getPublicAddrs4Gateway(gw *gatewayv1alpha2.Gateway) (gatewayv1alpha2.GatewayAddress, error) {
	r.log.V(4).Info("getPublicAddrs4Gateway", "Gateway", store.GetObjectKey(gw))

	for _, svc := range r.op.GetServices() {
		r.log.V(4).Info("considering service", "svc", store.GetObjectKey(svc))

		if r.isServiceAnnotated4Gateway(svc, gw) {
			r.log.V(4).Info("found service annotated for gateway", "gateway",
				gw.GetName(), "service", svc.GetName())

			// FIXME: fallback NodePort services
			if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
				r.log.V(2).Info("cannot obtian public IP for service",
					"gateway", gw.GetName(), "service", svc.GetName())
				continue
			}

			// check if at least on of the gateway's listener ports and one of the
			// service-ports match
			i, found := getServicePort(gw, svc)
			if !found {
				r.log.V(2).Info("service protocol/port does not match any listener "+
					"protocol/port", "gateway", gw.GetName(), "service",
					svc.GetName())
				continue
			}

			// get the public IPs
			if len(svc.Status.LoadBalancer.Ingress) == 0 {
				r.log.V(2).Info("cannot obtian public IP for service (ignoring)",
					"gateway", gw.GetName(), "service", svc.GetName())
				continue
			}

			// we have a valid index
			t := gatewayv1alpha2.AddressType("IPAddress")
			if i < len(svc.Status.LoadBalancer.Ingress) {
				r.log.V(4).Info("getPublicAddrs4Gateway: found public IP",
					"Gateway", store.GetObjectKey(gw), "result",
					svc.Status.LoadBalancer.Ingress[i].IP)

				return gatewayv1alpha2.GatewayAddress{
					Type:  &t,
					Value: svc.Status.LoadBalancer.Ingress[i].IP,
				}, nil
			}
			// fallback to the first addr we find
			r.log.V(4).Info("getPublicAddrs4Gateway: found public IP (fallback)",
				"Gateway", store.GetObjectKey(gw), "result",
				svc.Status.LoadBalancer.Ingress[0].IP)

			return gatewayv1alpha2.GatewayAddress{
				Type:  &t,
				Value: svc.Status.LoadBalancer.Ingress[0].IP,
			}, nil
		}
	}

	return gatewayv1alpha2.GatewayAddress{}, fmt.Errorf("load-balancer IP not found")
}

// we need the namespaced name!
func (r *Renderer) isServiceAnnotated4Gateway(svc *corev1.Service, gw *gatewayv1alpha2.Gateway) bool {
	r.log.V(4).Info("isServiceAnnotated4Gateway", "Service", store.GetObjectKey(svc),
		"Gateway", store.GetObjectKey(gw))

	as := svc.GetAnnotations()
	namespacedName := fmt.Sprintf("%s/%s", gw.GetNamespace(), gw.GetName())
	v, found := as[operator.GatewayAddressAnnotationKey]
	if found && v == namespacedName {
		r.log.V(4).Info("isServiceAnnotated4Gateway: service annotated foe gateway",
			"Service", store.GetObjectKey(svc), "Gateway", store.GetObjectKey(gw))
		return true
	}

	return false
}

// first matching listener-proto-port and service-proto-port pair
func getServicePort(gw *gatewayv1alpha2.Gateway, svc *corev1.Service) (int, bool) {
	for _, l := range gw.Spec.Listeners {
		for i, s := range svc.Spec.Ports {
			if strings.ToLower(string(l.Protocol)) == strings.ToLower(string(s.Protocol)) &&
				int32(l.Port) == s.Port {
				return i, true
			}
		}
	}
	return 0, false
}
