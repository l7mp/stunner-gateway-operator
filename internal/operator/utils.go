package operator

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
)

// GetManager returns the controller manager associated with this operator
func (o *Operator) GetManager() manager.Manager {
	return o.manager
}

// GetLogger returns the logger associated with this operator
func (o *Operator) GetLogger() logr.Logger {
	return o.logger
}

// GetControllerName returns the controller-name (as per GatewayClass.Spec.ControllerName)
// associated with this operator
func (o *Operator) GetControllerName() string {
	return o.controllerName
}

// GetGatewayClasses returns all GatewayClass objects from local storage
func (o *Operator) GetGatewayClasses() ([]*gatewayv1alpha2.GatewayClass, error) {
	ret := make([]*gatewayv1alpha2.GatewayClass, 0)

	if o.gatewayClassStore.Len() > 1 {
		return ret, fmt.Errorf("found multiple GatewayClasses in local store: "+
			"num: %d", o.gatewayClassStore.Len())
	}

	for _, o := range o.gatewayClassStore.Objects() {
		ret = append(ret, o.(*gatewayv1alpha2.GatewayClass))
	}

	return ret, nil
}

// GetGatewayConfig returns a named GatewayConfig object from local storage
func (o *Operator) GetGatewayConfig(nsName types.NamespacedName) *stunnerv1alpha1.GatewayConfig {
	obj := o.gatewayConfigStore.Get(nsName)
	if obj == nil {
		return nil
	}

	ret, ok := obj.(*stunnerv1alpha1.GatewayConfig)
	if ok != true {
		return nil
	}

	return ret
}

// GetGateway returns a named Gateway object from local storage
func (o *Operator) GetGateway(nsName types.NamespacedName) *gatewayv1alpha2.Gateway {
	obj := o.gatewayStore.Get(nsName)
	if obj == nil {
		return nil
	}

	ret, ok := obj.(*gatewayv1alpha2.Gateway)
	if ok != true {
		return nil
	}

	return ret
}

// GetGateways returns all Gateway objects from local storage
func (o *Operator) GetGateways() []*gatewayv1alpha2.Gateway {
	gws := []*gatewayv1alpha2.Gateway{}
	for _, obj := range o.gatewayStore.Objects() {
		gw, ok := obj.(*gatewayv1alpha2.Gateway)
		if ok != true {
			continue
		}
		gws = append(gws, gw)
	}

	return gws
}

// GetUDPRoute returns a named UDPRoute object from local storage
func (o *Operator) GetUDPRoute(nsName types.NamespacedName) *gatewayv1alpha2.UDPRoute {
	obj := o.udpRouteStore.Get(nsName)
	if obj == nil {
		return nil
	}

	ret, ok := obj.(*gatewayv1alpha2.UDPRoute)
	if ok != true {
		return nil
	}

	return ret
}

// GetUDPRoutes returns al UDPRoute objects from local storage
func (o *Operator) GetUDPRoutes() []*gatewayv1alpha2.UDPRoute {
	rs := []*gatewayv1alpha2.UDPRoute{}
	for _, obj := range o.udpRouteStore.Objects() {
		r, ok := obj.(*gatewayv1alpha2.UDPRoute)
		if ok != true {
			continue
		}
		rs = append(rs, r)
	}

	return rs
}

// GetService returns a named Service object from local storage
func (o *Operator) GetService(nsName types.NamespacedName) *corev1.Service {
	obj := o.serviceStore.Get(nsName)
	if obj == nil {
		return nil
	}

	ret, ok := obj.(*corev1.Service)
	if ok != true {
		return nil
	}

	return ret
}

// GetServices returns al Service objects from local storage
func (o *Operator) GetServices() []*corev1.Service {
	ss := []*corev1.Service{}
	for _, obj := range o.serviceStore.Objects() {
		s, ok := obj.(*corev1.Service)
		if ok != true {
			continue
		}
		ss = append(ss, s)
	}

	return ss
}

// AddGatewayClass adds a GatewayClass object to the local storage (this is used mainly for testing)
func (o *Operator) AddGatewayClass(gc *gatewayv1alpha2.GatewayClass) {
	o.gatewayClassStore.Upsert(gc)
}

// AddGatewayConfig adds a GatewayConfig object to the local storage (this is used mainly for testing)
func (o *Operator) AddGatewayConfig(gc *stunnerv1alpha1.GatewayConfig) {
	o.gatewayConfigStore.Upsert(gc)
}

// AddGateway adds a Gateway object to the local storage (this is used mainly for testing)
func (o *Operator) AddGateway(gw *gatewayv1alpha2.Gateway) {
	o.gatewayStore.Upsert(gw)
}

// AddUDPRoute adds a UDPRoute object to the local storage (this is used mainly for testing)
func (o *Operator) AddUDPRoute(r *gatewayv1alpha2.UDPRoute) {
	o.udpRouteStore.Upsert(r)
}

// AddService adds a Service object to the local storage (this is used mainly for testing)
func (o *Operator) AddService(s *corev1.Service) {
	o.serviceStore.Upsert(s)
}
