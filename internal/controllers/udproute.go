package controllers

import (
	"context"
	// "errors"
	// "fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	opdefault "github.com/l7mp/stunner-gateway-operator/api/config"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

const (
	serviceTCPRouteIndex = "serviceTCPRouteIndex"
	serviceUDPRouteIndex = "serviceUDPRouteIndex"
)

type udpRouteReconciler struct {
	client.Client
	eventCh chan event.Event
	log     logr.Logger
}

func RegisterUDPRouteController(mgr manager.Manager, ch chan event.Event, log logr.Logger) error {
	ctx := context.Background()
	r := &udpRouteReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("udproute-controller"),
	}

	c, err := controller.New("udproute", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created udproute controller")

	// watch UDPRoute objects
	if err := c.Watch(
		&source.Kind{Type: &gwapiv1a2.UDPRoute{}},
		&handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{},
	); err != nil {
		return err
	}
	r.log.Info("watching udproute objects")

	// index UDPRoute objects as per the referenced Services
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.UDPRoute{},
		serviceUDPRouteIndex, serviceUDPRouteIndexFunc); err != nil {
		return err
	}

	// watch Service objects referenced by one of our UDPRoutes
	if err := c.Watch(
		&source.Kind{Type: &corev1.Service{}},
		&handler.EnqueueRequestForObject{},
		// trigget when either a gateway-loadbalancer service (svc annotated as a
		// related-service for a gateway) or a backend-service changes
		predicate.Or(
			predicate.NewPredicateFuncs(r.validateBackendForReconcile),
			predicate.NewPredicateFuncs(r.validateLoadBalancerReconcile),
		),
	); err != nil {
		return err
	}
	r.log.Info("watching secret objects")

	// watch EndPoints object references by one of the ref'd Services
	if config.EnableEndpointDiscovery {
		if err := c.Watch(
			&source.Kind{Type: &corev1.Endpoints{}},
			&handler.EnqueueRequestForObject{},
			predicate.NewPredicateFuncs(r.validateBackendForReconcile),
		); err != nil {
			return err
		}
		r.log.Info("watching endpoint objects")
	}

	return nil
}

// Reconcile handles an update to a UDPRoute or a Service/Endpoints referenced by an UDPRoute.
func (r *udpRouteReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("udproute", req.String())
	log.Info("reconciling")

	routeList := []client.Object{}
	svcList := []client.Object{}
	endpointsList := []client.Object{}

	// find all related-services that we use as LoadBalancers for Gateways
	// TODO: this will fail on very large clusters (note that we cannot filter for annotations
	// at the server side, we must do the filtering on ALL services here)
	svcs := &corev1.ServiceList{}
	if err := r.List(ctx, svcs); err == nil {
		for _, svc := range svcs.Items {
			svc := svc
			if r.validateLoadBalancerReconcile(&svc) {
				svcList = append(svcList, &svc)
			}
		}
	}

	// find all UDPRoutes
	routes := &gwapiv1a2.UDPRouteList{}
	if err := r.List(ctx, routes); err != nil {
		r.log.Info("no UDPRoutes found")
		return reconcile.Result{}, err
	}

	for _, udproute := range routes.Items {
		udproute := udproute
		r.log.V(1).Info("processing UDPRoute", "name", store.GetObjectKey(&udproute))

		routeList = append(routeList, &udproute)

		for _, rule := range udproute.Spec.Rules {
			for _, ref := range rule.BackendRefs {
				ref := ref
				if (ref.Group != nil && *ref.Group != corev1.GroupName) ||
					(ref.Kind != nil && *ref.Kind != "Service") {
					continue
				}

				if svc := r.getServiceForBackend(ctx, &udproute, &ref); svc != nil {
					svcList = append(svcList, svc)
				}

				if config.EnableEndpointDiscovery {
					if e := r.getEndpointsForBackend(ctx, &udproute, &ref); e != nil {
						endpointsList = append(endpointsList, e)
					}
				}
			}
		}
	}

	store.UDPRoutes.Reset(routeList)
	r.log.V(2).Info("reset UDPRoute store", "udproutes", store.UDPRoutes.String())

	store.Services.Reset(svcList)
	r.log.V(2).Info("reset Service store", "services", store.Services.String())

	store.Endpoints.Reset(endpointsList)
	r.log.V(2).Info("reset Endpoints store", "endpoints", store.Endpoints.String())

	r.eventCh <- event.NewEventRender()

	return reconcile.Result{}, nil
}

// validateBackendForReconcile checks whether the Service belongs to a valid UDPRoute.
func (r *udpRouteReconciler) validateBackendForReconcile(o client.Object) bool {
	// are we given a service or an endpoints object?
	key := ""
	if svc, ok := o.(*corev1.Service); ok {
		key = store.GetObjectKey(svc)
	} else if e, ok := o.(*corev1.Endpoints); ok {
		// endpoints and services are of the same name
		key = store.GetObjectKey(e)
	} else {
		r.log.Info("unexpected object type, bypassing reconciliation", "object", store.GetObjectKey(o))
		return false
	}

	// find the routes referring to this service
	routeList := &gwapiv1a2.UDPRouteList{}
	if err := r.List(context.Background(), routeList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(serviceUDPRouteIndex, key),
	}); err != nil {
		r.log.Error(err, "unable to find associated udproutes", "service", key)
		return false
	}

	if len(routeList.Items) == 0 {
		return false
	}

	return true
}

// getServiceForBackend finds the Service associated with a backendRef
func (r *udpRouteReconciler) getServiceForBackend(ctx context.Context, udproute *gwapiv1a2.UDPRoute, ref *gwapiv1a2.BackendRef) *corev1.Service {
	svc := corev1.Service{}

	// if no explicit Service namespace is provided, use the UDPRoute namespace to lookup the
	// Service
	namespace := udproute.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	if err := r.Get(ctx,
		types.NamespacedName{Namespace: namespace, Name: string(ref.Name)},
		&svc,
	); err != nil {
		// not fatal
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "error getting Service", "namespace", namespace,
				"name", string(ref.Name))
			return nil
		}

		r.log.Info("no Service found for UDPRoute backend", "udproute",
			store.GetObjectKey(udproute), "namespace", namespace,
			"name", string(ref.Name))
		return nil
	}

	return &svc
}

// getEndpointsForBackend finds the Endpoints associated with a backendRef
func (r *udpRouteReconciler) getEndpointsForBackend(ctx context.Context, udproute *gwapiv1a2.UDPRoute, ref *gwapiv1a2.BackendRef) *corev1.Endpoints {
	e := corev1.Endpoints{}

	// if no explicit Endpoints namespace is provided, use the UDPRoute namespace to lookup the
	// Endpoints
	namespace := udproute.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	if err := r.Get(ctx,
		types.NamespacedName{Namespace: namespace, Name: string(ref.Name)},
		&e,
	); err != nil {
		// not fatal
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "error getting Endpoints", "namespace", namespace,
				"name", string(ref.Name))
			return nil
		}

		r.log.Info("no Endpoints found for UDPRoute backend", "udproute",
			store.GetObjectKey(udproute), "namespace", namespace,
			"name", string(ref.Name))
		return nil
	}

	return &e
}

func serviceUDPRouteIndexFunc(o client.Object) []string {
	udproute := o.(*gwapiv1a2.UDPRoute)
	var services []string

	for _, rule := range udproute.Spec.Rules {
		for _, backend := range rule.BackendRefs {
			if backend.Kind == nil || string(*backend.Kind) == "Service" {
				// if no explicit Service namespace is provided, use the UDPRoute
				// namespace to lookup the provided Service
				namespace := udproute.GetNamespace()
				if backend.Namespace != nil {
					namespace = string(*backend.Namespace)
				}

				services = append(services,
					types.NamespacedName{
						Namespace: namespace,
						Name:      string(backend.Name),
					}.String(),
				)
			}
		}
	}

	return services
}

// validateLoadBalancerReconcile checks whether a Service is annotated as a related-service for a
// gateway.
func (r *udpRouteReconciler) validateLoadBalancerReconcile(o client.Object) bool {
	_, found := o.GetAnnotations()[opdefault.DefaultRelatedGatewayAnnotationKey]
	return found
}
