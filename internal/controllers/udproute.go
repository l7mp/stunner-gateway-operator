package controllers

import (
	"context"
	// "errors"
	// "fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

const (
	serviceUDPRouteIndex           = "serviceUDPRouteIndex"
	serviceUDPRouteIndexV1A2       = "serviceUDPRouteIndexV1A2"
	staticServiceUDPRouteIndex     = "staticServiceUDPRouteIndex"
	staticServiceUDPRouteIndexV1A2 = "staticServiceUDPRouteIndexV1A2"
)

type udpRouteReconciler struct {
	client.Client
	eventCh     chan event.Event
	terminating bool
	log         logr.Logger
}

func NewUDPRouteController(mgr manager.Manager, ch chan event.Event, log logr.Logger) (Controller, error) {
	ctx := context.Background()
	r := &udpRouteReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("udproute-controller"),
	}

	c, err := controller.New("udproute", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	r.log.Info("created udproute controller")

	// watch UDPRoute objects
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &stnrgwv1.UDPRoute{}),
		&handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{},
	); err != nil {
		return nil, err
	}
	r.log.Info("watching udproute objects")

	// index UDPRoute objects as per the referenced Services
	if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1.UDPRoute{},
		serviceUDPRouteIndex, serviceUDPRouteIndexFunc); err != nil {
		return nil, err
	}

	// index UDPRoute objects as per the referenced StaticServices
	if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1.UDPRoute{},
		staticServiceUDPRouteIndex, staticServiceUDPRouteIndexFunc); err != nil {
		return nil, err
	}

	// watch UDPRouteV1A2 objects
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &gwapiv1a2.UDPRoute{}),
		&handler.EnqueueRequestForObject{},
		predicate.GenerationChangedPredicate{},
	); err != nil {
		return nil, err
	}
	r.log.Info("watching udproute objects")

	// index UDPRouteV1A2 objects as per the referenced Services
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.UDPRoute{},
		serviceUDPRouteIndexV1A2, serviceUDPRouteIndexFunc); err != nil {
		return nil, err
	}

	// index UDPRouteV1A2 objects as per the referenced StaticServices
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.UDPRoute{},
		staticServiceUDPRouteIndexV1A2, staticServiceUDPRouteIndexFunc); err != nil {
		return nil, err
	}

	// a label-selector predicate to select the loadbalancer services we are interested in
	loadBalancerPredicate, err := predicate.LabelSelectorPredicate(
		metav1.LabelSelector{
			MatchLabels: map[string]string{
				// LB services have both "app:stunner" and
				// "stunner.l7mp.io/owned-by:stunner" labels set, we use the app
				// label here
				opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
			},
		})
	if err != nil {
		return nil, err
	}

	// watch Service objects referenced by one of our UDPRoutes
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &corev1.Service{}),
		&handler.EnqueueRequestForObject{},
		// trigger when either a gateway-loadbalancer service (svc annotated as a
		// related-service for a gateway) or a backend-service changes
		predicate.Or(
			predicate.NewPredicateFuncs(r.validateBackendForReconcile),
			// predicate.NewPredicateFuncs(r.validateLoadBalancerReconcile),
			loadBalancerPredicate),
	); err != nil {
		return nil, err
	}
	r.log.Info("watching service objects")

	// watch EndPoints object references by one of the ref'd Services
	if config.EnableEndpointDiscovery {
		// try to set up a watch for EndpointSlices if the endpointslice controller was not
		// explicitly disabled on the command line
		if config.EndpointSliceAvailable {
			if err := c.Watch(
				source.Kind(mgr.GetCache(), &discoveryv1.EndpointSlice{}),
				&handler.EnqueueRequestForObject{},
				predicate.NewPredicateFuncs(r.validateEndpointSliceForReconcile),
			); err == nil {
				r.log.Info("watching endpointslice objects")
				config.EndpointSliceAvailable = true
			} else {
				r.log.Info("WARNING: EndpointSlice support diabled, falling back to " +
					"the Endpoints controller and disabling graceful backend " +
					"shutdown, see https://github.com/l7mp/stunner/issues/138")
				config.EndpointSliceAvailable = false
			}
		}

		// if EndpointSlices are still not available, fall back to wathing Endpoints
		if !config.EndpointSliceAvailable {
			if err := c.Watch(
				source.Kind(mgr.GetCache(), &corev1.Endpoints{}),
				&handler.EnqueueRequestForObject{},
				predicate.NewPredicateFuncs(r.validateBackendForReconcile),
			); err != nil {
				return nil, err
			}

			config.EndpointSliceAvailable = false
			r.log.Info("watching endpoint objects")
		}
	}

	// watch StaticService objects referenced by one of our UDPRoutes
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &stnrgwv1.StaticService{}),
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.validateStaticServiceForReconcile),
	); err != nil {
		return nil, err
	}
	r.log.Info("watching staticservice objects")

	return r, nil
}

// Reconcile handles an update to a UDPRoute or a Service/Endpoints referenced by an UDPRoute.
func (r *udpRouteReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("resource", req.String())

	if r.terminating {
		r.log.V(2).Info("controller terminating, suppressing reconciliation")
		return reconcile.Result{}, nil
	}

	log.Info("reconciling")
	routeList := []client.Object{}
	routeListV1A2 := []client.Object{}
	namespaceList := []client.Object{}
	svcList := []client.Object{}
	ssvcList := []client.Object{}
	endpointList := []client.Object{}

	// find all related-services that we use as LoadBalancers for Gateways (i.e., have label
	// "app:stunner")
	svcs := &corev1.ServiceList{}
	err := r.List(ctx, svcs, client.MatchingLabels{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue})
	if err == nil {
		for _, svc := range svcs.Items {
			svc := svc
			svcList = append(svcList, &svc)
		}
	}

	// find all UDPRoutes
	routes := &stnrgwv1.UDPRouteList{}
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

				// is this a static service?
				if store.IsReferenceStaticService(&ref) {
					if svc := r.getStaticServiceForBackend(ctx, &udproute, &ref); svc != nil {
						ssvcList = append(ssvcList, svc)
					}
					continue
				}

				if store.IsReferenceService(&ref) {
					if svc := r.getServiceForBackend(ctx, &udproute, &ref); svc != nil {
						r.log.V(2).Info("found service for udproute backend ref",
							"udproute", store.GetObjectKey(&udproute),
							"ref", store.DumpBackendRef(&ref),
							"svc", store.GetObjectKey(svc))
						svcList = append(svcList, svc)
					}

					if config.EnableEndpointDiscovery {
						if config.EndpointSliceAvailable {
							es := r.getEndpointSlicesForBackend(ctx, &udproute, &ref)
							endpointList = append(endpointList, es...)
						} else {
							if e := r.getEndpointsForBackend(ctx, &udproute, &ref); e != nil {
								endpointList = append(endpointList, e)
							}
						}
					}

					continue
				}
			}
		}

		nsName := udproute.GetNamespace()
		r.log.V(2).Info("looking for the namespace of UDPRoute", "name", nsName)
		namespace := corev1.Namespace{}
		if err := r.Get(ctx, types.NamespacedName{Name: nsName}, &namespace); err != nil {
			r.log.Error(err, "error getting namespace for udproute", "udproute",
				store.GetObjectKey(&udproute), "namespace-name", nsName)
			continue
		}

		namespaceList = append(namespaceList, &namespace)
	}

	// find all gwapi.v1alpha2 UDPRoutes and convert to our own UDPRoute format
	routesV1A2 := &gwapiv1a2.UDPRouteList{}
	if err := r.List(ctx, routesV1A2); err != nil {
		r.log.V(2).Info("no UDPRoutes V1A2 found")
		return reconcile.Result{}, err
	}

	for _, udprouteV1A2 := range routesV1A2.Items {
		udproute := stnrgwv1.ConvertV1A2UDPRouteToV1(&udprouteV1A2)
		r.log.V(1).Info("processing UDPRouteV1A2", "name", store.GetObjectKey(udproute))

		routeListV1A2 = append(routeListV1A2, udproute)

		for _, rule := range udproute.Spec.Rules {
			for _, ref := range rule.BackendRefs {
				ref := ref

				// is this a static service?
				if store.IsReferenceStaticService(&ref) {
					if svc := r.getStaticServiceForBackend(ctx, udproute, &ref); svc != nil {
						ssvcList = append(ssvcList, svc)
					}
					continue
				}

				if store.IsReferenceService(&ref) {
					if svc := r.getServiceForBackend(ctx, udproute, &ref); svc != nil {
						svcList = append(svcList, svc)
					}

					if config.EnableEndpointDiscovery {
						if config.EndpointSliceAvailable {
							es := r.getEndpointSlicesForBackend(ctx, udproute, &ref)
							if len(es) > 0 {
								endpointList = append(endpointList, es...)
							}
						} else {
							if e := r.getEndpointsForBackend(ctx, udproute, &ref); e != nil {
								endpointList = append(endpointList, e)
							}
						}
					}

					continue
				}
			}
		}

		nsName := udproute.GetNamespace()
		r.log.V(2).Info("looking for the namespace of UDPRoute", "name", nsName)
		namespace := corev1.Namespace{}
		if err := r.Get(ctx, types.NamespacedName{Name: nsName}, &namespace); err != nil {
			r.log.Error(err, "error getting namespace for udproute", "udproute",
				store.GetObjectKey(udproute), "namespace-name", nsName)
			continue
		}

		namespaceList = append(namespaceList, &namespace)
	}

	store.UDPRoutes.Reset(routeList)
	r.log.V(2).Info("reset UDPRoute store", "udproutes", store.UDPRoutes.String())

	store.UDPRoutesV1A2.Reset(routeListV1A2)
	r.log.V(2).Info("reset UDPRoute V1A2 store", "udproutes", store.UDPRoutesV1A2.String())

	store.Namespaces.Reset(namespaceList)
	r.log.V(2).Info("reset Namespace store", "namespaces", store.Namespaces.String())

	store.Services.Reset(svcList)
	r.log.V(2).Info("reset Service store", "services", store.Services.String())

	if config.EndpointSliceAvailable {
		store.EndpointSlices.Reset(endpointList)
		r.log.V(2).Info("reset EndpointSlice store", "endpointslices", store.EndpointSlices.String())
	} else {
		store.Endpoints.Reset(endpointList)
		r.log.V(2).Info("reset Endpoints store", "endpoints", store.Endpoints.String())
	}

	store.StaticServices.Reset(ssvcList)
	r.log.V(2).Info("reset StaticService store", "static-services", store.StaticServices.String())

	r.eventCh <- event.NewEventReconcile()

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
		return false
	}

	// find the routes referring to this service
	routeList := &stnrgwv1.UDPRouteList{}
	if err := r.List(context.Background(), routeList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(serviceUDPRouteIndex, key),
	}); err != nil {
		r.log.Error(err, "unable to find associated udproutes", "service", key)
		return false
	}

	// find V1A2 routes referring to this service
	routeListV1A2 := &gwapiv1a2.UDPRouteList{}
	if err := r.List(context.Background(), routeListV1A2, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(serviceUDPRouteIndexV1A2, key),
	}); err != nil {
		r.log.Error(err, "unable to find associated V1A2 udproutes", "service", key)
		return false
	}

	if len(routeList.Items) == 0 && len(routeListV1A2.Items) == 0 {
		r.log.Info("validateBackendForReconcile:", "udproute", "not found")
		return false
	}

	r.log.Info("validateBackendForReconcile:", "udproute", "found")
	return true
}

// validateEndpointSliceForReconcile checks whether an EndpointSlice belongs to a Service that
// belongs to a valid UDPRoute.
func (r *udpRouteReconciler) validateEndpointSliceForReconcile(o client.Object) bool {
	// are we given a proper EndpointSlice object?
	esl, ok := o.(*discoveryv1.EndpointSlice)
	if !ok {
		return false
	}

	// r.log.Info("validateEndpointSliceForReconcile:", "namespace/name", store.GetObjectKey(esl))

	// find the Service corresponding to this EndpointSlice
	// TODO: also check ownership
	svcName, ok := esl.GetLabels()[discoveryv1.LabelServiceName]
	if !ok {
		r.log.Info("validateEndpointSliceForReconcile:", "label", "not ok")
		return false
	}

	// r.log.Info("validateEndpointSliceForReconcile:", "label", "ok")

	svc := &corev1.Service{}
	if err := r.Get(context.Background(), types.NamespacedName{
		Namespace: esl.GetNamespace(),
		Name:      svcName,
	}, svc); err != nil {
		// not fatal
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "error getting Service for EndpointSlice",
				"namespace", esl.GetNamespace(),
				"name", svcName)
		}
		return false
	}

	// r.log.Info("validateEndpointSliceForReconcile:", "svc", "found")

	return r.validateBackendForReconcile(svc)
}

// validateStaticServiceForReconcile checks whether a Static Service belongs to a valid UDPRoute.
func (r *udpRouteReconciler) validateStaticServiceForReconcile(o client.Object) bool {
	// are we given a service or an endpoints object?
	key := ""
	if svc, ok := o.(*stnrgwv1.StaticService); ok {
		key = store.GetObjectKey(svc)
	} else {
		return false
	}

	// find the routes referring to this static service
	routeList := &stnrgwv1.UDPRouteList{}
	if err := r.List(context.Background(), routeList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(staticServiceUDPRouteIndex, key),
	}); err != nil {
		r.log.Error(err, "unable to find associated udproutes", "static-service", key)
		return false
	}

	routeListV1A2 := &gwapiv1a2.UDPRouteList{}
	if err := r.List(context.Background(), routeListV1A2, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(staticServiceUDPRouteIndexV1A2, key),
	}); err != nil {
		r.log.Error(err, "unable to find associated V1A2 udproutes", "static-service", key)
		return false
	}

	if len(routeList.Items) == 0 && len(routeListV1A2.Items) == 0 {
		return false
	}

	return true
}

// getServiceForBackend finds the Service associated with a backendRef
func (r *udpRouteReconciler) getServiceForBackend(ctx context.Context, udproute *stnrgwv1.UDPRoute, ref *stnrgwv1.BackendRef) *corev1.Service {
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

// getEndpointSlicesForBackend finds all EndpointSlices associated with a backendRef
func (r *udpRouteReconciler) getEndpointSlicesForBackend(ctx context.Context, udproute *stnrgwv1.UDPRoute, ref *stnrgwv1.BackendRef) []client.Object {
	// if no explicit Endpoints namespace is provided, use the UDPRoute namespace to lookup the
	// Endpoints
	namespace := udproute.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	// find the EndpointSlicce corresponding to the backend service
	esls := discoveryv1.EndpointSliceList{}
	labelSelector := labels.SelectorFromSet(labels.Set{discoveryv1.LabelServiceName: string(ref.Name)})
	listOptions := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	if err := r.List(ctx, &esls, listOptions); err != nil {
		r.log.Error(err, "error getting endpointslices for backend service",
			"namespace", namespace, "backend-name", string(ref.Name))
		return []client.Object{}
	}

	es := make([]client.Object, len(esls.Items))
	for i := range esls.Items {
		es[i] = &esls.Items[i]
	}

	if len(es) == 0 {
		r.log.Info("no endpointslice found for UDPRoute backend", "udproute",
			store.GetObjectKey(udproute), "backend-ref",
			store.DumpBackendRef(ref))
	}

	return es
}

// getEndpointsForBackend finds the Endpoints associated with a backendRef
func (r *udpRouteReconciler) getEndpointsForBackend(ctx context.Context, udproute *stnrgwv1.UDPRoute, ref *stnrgwv1.BackendRef) client.Object {
	// if no explicit Endpoints namespace is provided, use the UDPRoute namespace to lookup the
	// Endpoints
	namespace := udproute.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	ep := corev1.Endpoints{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: string(ref.Name)}, &ep); err != nil {
		// not fatal
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "error getting Endpoints", "namespace", namespace, "name",
				string(ref.Name))
		}

		r.log.Info("no endpoint found for UDPRoute backend", "udproute",
			store.GetObjectKey(udproute), "namespace", namespace, "name",
			string(ref.Name))

		return nil
	}

	return &ep
}

// getStaticServiceForBackend finds the StaticService associated with a backendRef
func (r *udpRouteReconciler) getStaticServiceForBackend(ctx context.Context, udproute *stnrgwv1.UDPRoute, ref *stnrgwv1.BackendRef) *stnrgwv1.StaticService {
	svc := stnrgwv1.StaticService{}

	// if no explicit StaticService namespace is provided, use the UDPRoute namespace to lookup the
	// StaticService
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
			r.log.Error(err, "error getting StaticService", "namespace", namespace,
				"name", string(ref.Name))
			return nil
		}

		r.log.Info("no StaticService found for UDPRoute backend", "udproute",
			store.GetObjectKey(udproute), "namespace", namespace,
			"name", string(ref.Name))
		return nil
	}

	return &svc
}

func serviceUDPRouteIndexFunc(o client.Object) []string {
	var udproute *stnrgwv1.UDPRoute
	switch ro := o.(type) {
	case *gwapiv1a2.UDPRoute:
		udproute = stnrgwv1.ConvertV1A2UDPRouteToV1(ro)
	case *stnrgwv1.UDPRoute:
		udproute = ro
	default:
		return []string{}
	}

	var services []string
	for _, rule := range udproute.Spec.Rules {
		for _, backend := range rule.BackendRefs {
			if !store.IsReferenceService(&backend) {
				continue
			}

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

func staticServiceUDPRouteIndexFunc(o client.Object) []string {
	var udproute *stnrgwv1.UDPRoute
	switch ro := o.(type) {
	case *gwapiv1a2.UDPRoute:
		udproute = stnrgwv1.ConvertV1A2UDPRouteToV1(ro)
	case *stnrgwv1.UDPRoute:
		udproute = ro
	default:
		return []string{}
	}

	var staticServices []string
	for _, rule := range udproute.Spec.Rules {
		for _, backend := range rule.BackendRefs {
			backend := backend

			if !store.IsReferenceStaticService(&backend) {
				continue
			}

			// if no explicit StaticService namespace is provided, use the UDPRoute
			// namespace to lookup the provided static service
			namespace := udproute.GetNamespace()
			if backend.Namespace != nil {
				namespace = string(*backend.Namespace)
			}

			staticServices = append(staticServices,
				types.NamespacedName{
					Namespace: namespace,
					Name:      string(backend.Name),
				}.String(),
			)
		}
	}

	return staticServices
}

func (r *udpRouteReconciler) Terminate() {
	r.terminating = true
}
