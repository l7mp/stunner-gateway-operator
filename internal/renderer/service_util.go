package renderer

import (
	// "fmt"
	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
)

func (r *Renderer) getPublicAddrs4Gateway(gw *gatewayv1alpha2.Gateway) []gatewayv1alpha2.GatewayAddress {
	addrs := make([]gatewayv1alpha2.GatewayAddress, 0)
	for _, svc := range r.op.GetServices() {
		if r.isServiceAnnotated4Gateway(svc, gw) {
			r.log.V(3).Info("found service annotated for gateway", "gateway",
				gw.GetName(), "service", svc.GetName())

			// get the public IPs
			if len(svc.Status.LoadBalancer.Ingress) == 0 {
				r.log.V(1).Info("cannot obtian public IP for service (ignoring)",
					"gateway", gw.GetName(), "service", svc.GetName())
				continue
			}

			for _, i := range svc.Status.LoadBalancer.Ingress {
				t := gatewayv1alpha2.AddressType("IPAddress")
				addrs = append(addrs, gatewayv1alpha2.GatewayAddress{
					Type:  &t,
					Value: i.IP,
				})
			}
		}
	}

	return addrs
}

func (r *Renderer) isServiceAnnotated4Gateway(svc *corev1.Service, gw *gatewayv1alpha2.Gateway) bool {
	as := svc.GetAnnotations()
	v, found := as[operator.GatewayAddressAnnotationKey]
	if found && v == gw.GetName() {
		return true
	}

	return false
}
