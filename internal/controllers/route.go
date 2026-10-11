package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

const (
	serviceUDPRouteIndex            = "serviceUDPRouteIndex"
	serviceUDPRouteIndexGwAPI       = "serviceUDPRouteIndexGwAPI"
	staticServiceUDPRouteIndex      = "staticServiceUDPRouteIndex"
	staticServiceUDPRouteIndexGwAPI = "staticServiceUDPRouteIndexGwAPI"
	serviceTCPRouteIndex            = "serviceTCPRouteIndex"
	serviceTCPRouteIndexGwAPI       = "serviceTCPRouteIndexGwAPI"
	staticServiceTCPRouteIndex      = "staticServiceTCPRouteIndex"
	staticServiceTCPRouteIndexGwAPI = "staticServiceTCPRouteIndexGwAPI"
	serviceTURNRouteIndex           = "serviceTURNRouteIndex"
	staticServiceTURNRouteIndex     = "staticServiceTURNRouteIndex"
	secretTURNRouteIndex            = "secretTURNRouteIndex"
)

type routeReconciler struct {
	client.Client
	eventCh chan<- event.Event
	// udpRouteVersion and tcpRouteVersion hold the Gateway API version at which the official
	// route resources are watched (empty if the CRD is not installed): the graduated v1
	// version is preferred over the deprecated v1alpha2.
	udpRouteVersion, tcpRouteVersion string
	// tcpRouteServed and turnRouteServed are true if the cluster serves the TCPRoute and the
	// TURNRoute CRD, respectively.
	tcpRouteServed, turnRouteServed bool
	log                             logr.Logger
}

// routeBackends accumulates the backend objects referenced by the reconciled routes.
type routeBackends struct {
	svcList, ssvcList, endpointList, namespaceList, secretList []client.Object
}

func NewRouteController(mgr manager.Manager, ch chan<- event.Event, log logr.Logger) (Controller, error) {
	ctx := context.Background()
	r := &routeReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("route-controller"),
	}

	c, err := controller.New("route", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	r.log.Info("Created route controller")

	// watch UDPRoute objects
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &stnrgwv1.UDPRoute{},
			&handler.TypedEnqueueRequestForObject[*stnrgwv1.UDPRoute]{},
			predicate.TypedGenerationChangedPredicate[*stnrgwv1.UDPRoute]{}),
	); err != nil {
		return nil, err
	}
	r.log.Info("Watching UDPRoute objects")

	// index UDPRoute objects as per the referenced Services and StaticServices
	if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1.UDPRoute{},
		serviceUDPRouteIndex, serviceRouteIndexFunc); err != nil {
		return nil, err
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1.UDPRoute{},
		staticServiceUDPRouteIndex, staticServiceRouteIndexFunc); err != nil {
		return nil, err
	}

	// watch TCPRoute objects when the cluster serves the CRD
	r.tcpRouteServed, err = r.isRouteResourceServed(mgr, &stnrgwv1.TCPRoute{}, "tcproutes")
	if err != nil {
		return nil, err
	}

	if r.tcpRouteServed {
		if err := c.Watch(
			source.Kind(mgr.GetCache(), &stnrgwv1.TCPRoute{},
				&handler.TypedEnqueueRequestForObject[*stnrgwv1.TCPRoute]{},
				predicate.TypedGenerationChangedPredicate[*stnrgwv1.TCPRoute]{}),
		); err != nil {
			return nil, err
		}
		r.log.Info("Watching TCPRoute objects")

		// index TCPRoute objects as per the referenced Services and StaticServices
		if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1.TCPRoute{},
			serviceTCPRouteIndex, serviceRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1.TCPRoute{},
			staticServiceTCPRouteIndex, staticServiceRouteIndexFunc); err != nil {
			return nil, err
		}
	} else {
		r.log.Info("The TCPRoute CRD is not served by the cluster: TCPRoutes will be " +
			"ignored entirely. Apply the stunner.l7mp.io CRDs from the chart to enable " +
			"TCP routing; note that helm upgrade does not update CRDs")
	}

	// watch TURNRoute objects, and the Secrets they reference, when the cluster serves the CRD
	r.turnRouteServed, err = r.isRouteResourceServed(mgr, &stnrgwv1a1.TURNRoute{}, "turnroutes")
	if err != nil {
		return nil, err
	}
	config.TURNRouteServed = r.turnRouteServed

	if r.turnRouteServed {
		if err := c.Watch(
			source.Kind(mgr.GetCache(), &stnrgwv1a1.TURNRoute{},
				&handler.TypedEnqueueRequestForObject[*stnrgwv1a1.TURNRoute]{},
				predicate.TypedGenerationChangedPredicate[*stnrgwv1a1.TURNRoute]{}),
		); err != nil {
			return nil, err
		}
		r.log.Info("Watching TURNRoute objects")

		// index TURNRoute objects as per the referenced Services, StaticServices and Secrets
		if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1a1.TURNRoute{},
			serviceTURNRouteIndex, serviceRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1a1.TURNRoute{},
			staticServiceTURNRouteIndex, staticServiceRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1a1.TURNRoute{},
			secretTURNRouteIndex, secretTURNRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := c.Watch(
			source.Kind(mgr.GetCache(), &corev1.Secret{},
				&handler.TypedEnqueueRequestForObject[*corev1.Secret]{},
				predicate.NewTypedPredicateFuncs[*corev1.Secret](r.validateSecretForReconcile)),
		); err != nil {
			return nil, err
		}
		r.log.Info("Watching Secret objects referenced by TURNRoutes")
	} else {
		r.log.Info("The TURNRoute CRD is not served by the cluster: TURNRoutes will be " +
			"ignored entirely. Apply the stunner.l7mp.io CRDs from the chart to enable " +
			"TURN tunnels; note that helm upgrade does not update CRDs")
	}

	// watch the official Gateway API UDPRoute objects at the served version (only when the
	// CRD is loaded, preferring v1 over the deprecated v1alpha2 -- never both, as the two
	// versions represent the same objects)
	r.udpRouteVersion, err = r.gwAPIRouteVersion(mgr, &gwapiv1.UDPRoute{}, &gwapiv1a2.UDPRoute{}, "udproutes")
	if err != nil {
		return nil, err
	}
	config.GwAPIUDPRouteVersion = r.udpRouteVersion

	switch r.udpRouteVersion {
	case config.GwAPIVersionV1:
		if err := c.Watch(
			source.Kind(mgr.GetCache(), &gwapiv1.UDPRoute{},
				&handler.TypedEnqueueRequestForObject[*gwapiv1.UDPRoute]{},
				predicate.TypedGenerationChangedPredicate[*gwapiv1.UDPRoute]{}),
		); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1.UDPRoute{},
			serviceUDPRouteIndexGwAPI, serviceRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1.UDPRoute{},
			staticServiceUDPRouteIndexGwAPI, staticServiceRouteIndexFunc); err != nil {
			return nil, err
		}
		r.log.Info("Watching Gateway API v1 UDPRoute objects")
	case config.GwAPIVersionV1A2:
		if err := c.Watch(
			source.Kind(mgr.GetCache(), &gwapiv1a2.UDPRoute{},
				&handler.TypedEnqueueRequestForObject[*gwapiv1a2.UDPRoute]{},
				predicate.TypedGenerationChangedPredicate[*gwapiv1a2.UDPRoute]{}),
		); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.UDPRoute{},
			serviceUDPRouteIndexGwAPI, serviceRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.UDPRoute{},
			staticServiceUDPRouteIndexGwAPI, staticServiceRouteIndexFunc); err != nil {
			return nil, err
		}
		r.log.Info("Watching Gateway API v1alpha2 UDPRoute objects")
	default:
		r.log.V(1).Info("Gateway API UDPRoute CRD not available, skipping")
	}

	// watch the official Gateway API TCPRoute objects at the served version
	r.tcpRouteVersion, err = r.gwAPIRouteVersion(mgr, &gwapiv1.TCPRoute{}, &gwapiv1a2.TCPRoute{}, "tcproutes")
	if err != nil {
		return nil, err
	}
	config.GwAPITCPRouteVersion = r.tcpRouteVersion

	switch r.tcpRouteVersion {
	case config.GwAPIVersionV1:
		if err := c.Watch(
			source.Kind(mgr.GetCache(), &gwapiv1.TCPRoute{},
				&handler.TypedEnqueueRequestForObject[*gwapiv1.TCPRoute]{},
				predicate.TypedGenerationChangedPredicate[*gwapiv1.TCPRoute]{}),
		); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1.TCPRoute{},
			serviceTCPRouteIndexGwAPI, serviceRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1.TCPRoute{},
			staticServiceTCPRouteIndexGwAPI, staticServiceRouteIndexFunc); err != nil {
			return nil, err
		}
		r.log.Info("Watching Gateway API v1 TCPRoute objects")
	case config.GwAPIVersionV1A2:
		if err := c.Watch(
			source.Kind(mgr.GetCache(), &gwapiv1a2.TCPRoute{},
				&handler.TypedEnqueueRequestForObject[*gwapiv1a2.TCPRoute]{},
				predicate.TypedGenerationChangedPredicate[*gwapiv1a2.TCPRoute]{}),
		); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.TCPRoute{},
			serviceTCPRouteIndexGwAPI, serviceRouteIndexFunc); err != nil {
			return nil, err
		}

		if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.TCPRoute{},
			staticServiceTCPRouteIndexGwAPI, staticServiceRouteIndexFunc); err != nil {
			return nil, err
		}
		r.log.Info("Watching Gateway API v1alpha2 TCPRoute objects")
	default:
		r.log.V(1).Info("Gateway API TCPRoute CRD not available, skipping")
	}

	// a label-selector predicate to select the loadbalancer services we are interested in
	loadBalancerPredicate, err := ServiceLabelSelectorPredicate(
		metav1.LabelSelector{
			MatchLabels: map[string]string{
				// LB services carry the "stunner.l7mp.io/owned-by:stunner" label
				opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
			},
		})
	if err != nil {
		return nil, err
	}

	// watch Service objects referenced by one of our routes
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &corev1.Service{},
			&handler.TypedEnqueueRequestForObject[*corev1.Service]{},
			// trigger when either a gateway-loadbalancer service (svc annotated as a
			// related-service for a gateway) or a backend-service changes
			predicate.Or(
				predicate.NewTypedPredicateFuncs[*corev1.Service](r.validateBackendServiceForReconcile),
				loadBalancerPredicate)),
	); err != nil {
		return nil, err
	}
	r.log.Info("Watching Service objects")

	// watch EndpointSlices of the referenced Services
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &discoveryv1.EndpointSlice{},
			&handler.TypedEnqueueRequestForObject[*discoveryv1.EndpointSlice]{},
			predicate.NewTypedPredicateFuncs[*discoveryv1.EndpointSlice](r.validateEndpointSliceForReconcile)),
	); err != nil {
		return nil, err
	}
	r.log.Info("Watching EndpointSlice objects")

	// watch StaticService objects referenced by one of our routes
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &stnrgwv1.StaticService{},
			&handler.TypedEnqueueRequestForObject[*stnrgwv1.StaticService]{},
			predicate.NewTypedPredicateFuncs[*stnrgwv1.StaticService](r.validateStaticServiceForReconcile)),
	); err != nil {
		return nil, err
	}
	r.log.Info("Watching StaticService objects")

	return r, nil
}

// Reconcile handles an update to a route or a Service/EndpointSlice referenced by a route.
func (r *routeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("resource", req.String())

	log.Info("Reconciling")
	udpRouteList := []client.Object{}
	udpRouteListGwAPI := []client.Object{}
	tcpRouteList := []client.Object{}
	tcpRouteListGwAPI := []client.Object{}
	turnRouteList := []client.Object{}
	backends := routeBackends{}

	// find all related-services that we use as LoadBalancers for Gateways (i.e., have the
	// owned-by label)
	svcs := &corev1.ServiceList{}
	err := r.List(ctx, svcs, client.MatchingLabels{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue})
	if err == nil {
		for _, svc := range svcs.Items {
			svc := svc
			backends.svcList = append(backends.svcList, &svc)
		}
	}

	// find all UDPRoutes
	udpRoutes := &stnrgwv1.UDPRouteList{}
	if err := r.List(ctx, udpRoutes); err != nil {
		r.log.Info("No UDPRoutes found")
	} else {
		for i := range udpRoutes.Items {
			ro := &udpRoutes.Items[i]
			r.log.V(1).Info("Processing UDPRoute", "name", store.GetObjectKey(ro))

			udpRouteList = append(udpRouteList, ro)
			r.collectBackends(ctx, ro, ro.Spec.Rules, &backends)
		}
	}

	// find all official Gateway API UDPRoutes and convert to our own UDPRoute format
	switch r.udpRouteVersion {
	case config.GwAPIVersionV1:
		routesV1 := &gwapiv1.UDPRouteList{}
		if err := r.List(ctx, routesV1); err != nil {
			r.log.V(2).Info("No Gateway API UDPRoute resources found")
			return reconcile.Result{}, err
		}

		for i := range routesV1.Items {
			ro := stnrgwv1.ConvertV1UDPRouteToStnrV1(&routesV1.Items[i])
			r.log.V(1).Info("Processing Gateway API UDPRoute", "name", store.GetObjectKey(ro))

			udpRouteListGwAPI = append(udpRouteListGwAPI, ro)
			r.collectBackends(ctx, ro, ro.Spec.Rules, &backends)
		}
	case config.GwAPIVersionV1A2:
		routesV1A2 := &gwapiv1a2.UDPRouteList{}
		if err := r.List(ctx, routesV1A2); err != nil {
			r.log.V(2).Info("No Gateway API UDPRoute resources found")
			return reconcile.Result{}, err
		}

		for i := range routesV1A2.Items {
			ro := stnrgwv1.ConvertV1A2UDPRouteToStnrV1(&routesV1A2.Items[i])
			r.log.V(1).Info("Processing Gateway API UDPRoute", "name", store.GetObjectKey(ro))

			udpRouteListGwAPI = append(udpRouteListGwAPI, ro)
			r.collectBackends(ctx, ro, ro.Spec.Rules, &backends)
		}
	}

	// find all TCPRoutes
	if r.tcpRouteServed {
		tcpRoutes := &stnrgwv1.TCPRouteList{}
		if err := r.List(ctx, tcpRoutes); err != nil {
			r.log.Info("No TCPRoutes found")
		} else {
			for i := range tcpRoutes.Items {
				ro := &tcpRoutes.Items[i]
				r.log.V(1).Info("Processing TCPRoute", "name", store.GetObjectKey(ro))

				tcpRouteList = append(tcpRouteList, ro)
				r.collectBackends(ctx, ro, ro.Spec.Rules, &backends)
			}
		}
	}

	// find all official Gateway API TCPRoutes and convert to our own TCPRoute format
	switch r.tcpRouteVersion {
	case config.GwAPIVersionV1:
		routesV1 := &gwapiv1.TCPRouteList{}
		if err := r.List(ctx, routesV1); err != nil {
			r.log.V(2).Info("No Gateway API TCPRoute resources found")
			return reconcile.Result{}, err
		}

		for i := range routesV1.Items {
			ro := stnrgwv1.ConvertV1TCPRouteToStnrV1(&routesV1.Items[i])
			r.log.V(1).Info("Processing Gateway API TCPRoute", "name", store.GetObjectKey(ro))

			tcpRouteListGwAPI = append(tcpRouteListGwAPI, ro)
			r.collectBackends(ctx, ro, ro.Spec.Rules, &backends)
		}
	case config.GwAPIVersionV1A2:
		routesV1A2 := &gwapiv1a2.TCPRouteList{}
		if err := r.List(ctx, routesV1A2); err != nil {
			r.log.V(2).Info("No Gateway API TCPRoute resources found")
			return reconcile.Result{}, err
		}

		for i := range routesV1A2.Items {
			ro := stnrgwv1.ConvertV1A2TCPRouteToStnrV1(&routesV1A2.Items[i])
			r.log.V(1).Info("Processing Gateway API TCPRoute", "name", store.GetObjectKey(ro))

			tcpRouteListGwAPI = append(tcpRouteListGwAPI, ro)
			r.collectBackends(ctx, ro, ro.Spec.Rules, &backends)
		}
	}

	// find all TURNRoutes and the Secrets their tunnels authenticate with
	if r.turnRouteServed {
		turnRoutes := &stnrgwv1a1.TURNRouteList{}
		if err := r.List(ctx, turnRoutes); err != nil {
			r.log.Info("No TURNRoutes found")
		} else {
			for i := range turnRoutes.Items {
				ro := &turnRoutes.Items[i]
				r.log.V(1).Info("Processing TURNRoute", "name", store.GetObjectKey(ro))

				turnRouteList = append(turnRouteList, ro)
				r.collectBackends(ctx, ro, ro.Spec.Rules, &backends)
				if n, ok := tunnelAuthSecretName(ro); ok {
					secret := &corev1.Secret{}
					if err := r.Get(ctx, n, secret); err != nil {
						r.log.Info("No Secret found for TURNRoute tunnel auth", "route",
							store.GetObjectKey(ro), "secret", n.String(), "error", err.Error())
					} else {
						backends.secretList = append(backends.secretList, secret)
					}
				}
			}
		}
	}

	store.UDPRoutes.Reset(udpRouteList)
	r.log.V(2).Info("Reset UDPRoute store", "udproutes", store.UDPRoutes.String())

	store.UDPRoutesGwAPI.Reset(udpRouteListGwAPI)
	r.log.V(2).Info("Reset Gateway API UDPRoute store", "udproutes", store.UDPRoutesGwAPI.String())

	store.TCPRoutes.Reset(tcpRouteList)
	r.log.V(2).Info("Reset TCPRoute store", "tcproutes", store.TCPRoutes.String())

	store.TCPRoutesGwAPI.Reset(tcpRouteListGwAPI)
	r.log.V(2).Info("Reset Gateway API TCPRoute store", "tcproutes", store.TCPRoutesGwAPI.String())

	store.TURNRoutes.Reset(turnRouteList)
	r.log.V(2).Info("Reset TURNRoute store", "turnroutes", store.TURNRoutes.String())

	store.TunnelAuthSecrets.Reset(backends.secretList)
	r.log.V(2).Info("Reset tunnel auth Secret store", "secrets", store.TunnelAuthSecrets.String())

	store.Namespaces.Reset(backends.namespaceList)
	r.log.V(2).Info("Reset Namespace store", "namespaces", store.Namespaces.String())

	store.Services.Reset(backends.svcList)
	r.log.V(2).Info("Reset Service store", "services", store.Services.String())

	store.EndpointSlices.Reset(backends.endpointList)
	r.log.V(2).Info("Reset EndpointSlice store", "endpointslices", store.EndpointSlices.String())

	store.StaticServices.Reset(backends.ssvcList)
	r.log.V(2).Info("Reset StaticService store", "static-services", store.StaticServices.String())

	event.Send(ctx, r.eventCh, event.NewEventReconcile(string(r.Name())))

	return reconcile.Result{}, nil
}

// collectBackends gathers the backend Services, StaticServices, EndpointSlices and the
// namespace referenced by a route into the accumulator.
func (r *routeReconciler) collectBackends(ctx context.Context, ro client.Object, rules []stnrgwv1.RouteRule, acc *routeBackends) {
	for _, rule := range rules {
		for _, ref := range rule.BackendRefs {
			ref := ref

			// is this a static service?
			if store.IsReferenceStaticService(&ref) {
				if svc := r.getStaticServiceForBackend(ctx, ro, &ref); svc != nil {
					acc.ssvcList = append(acc.ssvcList, svc)
				}
				continue
			}

			if store.IsReferenceService(&ref) {
				if svc := r.getServiceForBackend(ctx, ro, &ref); svc != nil {
					r.log.V(2).Info("Found service for route backend ref",
						"route", store.GetObjectKey(ro),
						"ref", store.DumpBackendRef(&ref),
						"svc", store.GetObjectKey(svc))
					acc.svcList = append(acc.svcList, svc)
				}

				es := r.getEndpointSlicesForBackend(ctx, ro, &ref)
				acc.endpointList = append(acc.endpointList, es...)

				continue
			}
		}
	}

	nsName := ro.GetNamespace()
	r.log.V(2).Info("Looking for the namespace of route", "name", nsName)
	namespace := corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: nsName}, &namespace); err != nil {
		r.log.Error(err, "Error getting namespace for route", "route",
			store.GetObjectKey(ro), "namespace-name", nsName)
		return
	}

	acc.namespaceList = append(acc.namespaceList, &namespace)
}

func (r *routeReconciler) validateBackendServiceForReconcile(svc *corev1.Service) bool {
	return r.validateBackendForReconcile(store.GetObjectKey(svc), serviceUDPRouteIndex,
		serviceUDPRouteIndexGwAPI, serviceTCPRouteIndex, serviceTCPRouteIndexGwAPI,
		serviceTURNRouteIndex)
}

func (r *routeReconciler) validateStaticServiceForReconcile(staticSvc *stnrgwv1.StaticService) bool {
	return r.validateBackendForReconcile(store.GetObjectKey(staticSvc), staticServiceUDPRouteIndex,
		staticServiceUDPRouteIndexGwAPI, staticServiceTCPRouteIndex, staticServiceTCPRouteIndexGwAPI,
		staticServiceTURNRouteIndex)
}

// validateSecretForReconcile checks whether a Secret is the auth Secret of a TURNRoute.
func (r *routeReconciler) validateSecretForReconcile(secret *corev1.Secret) bool {
	turnRouteList := &stnrgwv1a1.TURNRouteList{}
	if err := r.List(context.Background(), turnRouteList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(secretTURNRouteIndex, store.GetObjectKey(secret)),
	}); err != nil {
		r.log.Error(err, "Unable to find associated TURNRoute", "secret", store.GetObjectKey(secret))
		return false
	}
	return len(turnRouteList.Items) > 0
}

// validateBackendForReconcile checks whether the Service or StaticService belongs to a valid
// route. Uses the indexers in the argument.
func (r *routeReconciler) validateBackendForReconcile(key, udpIndex, udpIndexGwAPI, tcpIndex, tcpIndexGwAPI, turnIndex string) bool {
	routeNum := 0

	// find the TURNRoutes referring to this service
	if r.turnRouteServed {
		turnRouteList := &stnrgwv1a1.TURNRouteList{}
		if err := r.List(context.Background(), turnRouteList, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(turnIndex, key),
		}); err != nil {
			r.log.Error(err, "Unable to find associated TURNRoute", "service", key)
		} else {
			routeNum += len(turnRouteList.Items)
		}
	}

	// find the UDPRoutes referring to this service
	udpRouteList := &stnrgwv1.UDPRouteList{}
	if err := r.List(context.Background(), udpRouteList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(udpIndex, key),
	}); err != nil {
		r.log.Error(err, "Unable to find associated UDPRoute", "service", key)
	} else {
		routeNum += len(udpRouteList.Items)
	}

	// find official Gateway API UDPRoutes referring to this service
	var udpRouteListGwAPI client.ObjectList
	switch r.udpRouteVersion {
	case config.GwAPIVersionV1:
		udpRouteListGwAPI = &gwapiv1.UDPRouteList{}
	case config.GwAPIVersionV1A2:
		udpRouteListGwAPI = &gwapiv1a2.UDPRouteList{}
	}
	if udpRouteListGwAPI != nil {
		if err := r.List(context.Background(), udpRouteListGwAPI, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(udpIndexGwAPI, key),
		}); err != nil {
			r.log.Error(err, "Unable to find associated Gateway API UDPRoute", "service", key)
		} else if items, err := meta.ExtractList(udpRouteListGwAPI); err == nil {
			routeNum += len(items)
		}
	}

	// find the TCPRoutes referring to this service
	if r.tcpRouteServed {
		tcpRouteList := &stnrgwv1.TCPRouteList{}
		if err := r.List(context.Background(), tcpRouteList, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(tcpIndex, key),
		}); err != nil {
			r.log.Error(err, "Unable to find associated TCPRoute", "service", key)
		} else {
			routeNum += len(tcpRouteList.Items)
		}
	}

	// find official Gateway API TCPRoutes referring to this service
	var tcpRouteListGwAPI client.ObjectList
	switch r.tcpRouteVersion {
	case config.GwAPIVersionV1:
		tcpRouteListGwAPI = &gwapiv1.TCPRouteList{}
	case config.GwAPIVersionV1A2:
		tcpRouteListGwAPI = &gwapiv1a2.TCPRouteList{}
	}
	if tcpRouteListGwAPI != nil {
		if err := r.List(context.Background(), tcpRouteListGwAPI, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(tcpIndexGwAPI, key),
		}); err != nil {
			r.log.Error(err, "Unable to find associated Gateway API TCPRoute", "service", key)
		} else if items, err := meta.ExtractList(tcpRouteListGwAPI); err == nil {
			routeNum += len(items)
		}
	}

	resStr := "not found"
	if routeNum > 0 {
		resStr = fmt.Sprintf("found %d routes", routeNum)
	}

	r.log.Info("Validating backend", "key", key, "route", resStr)

	return routeNum != 0
}

// validateEndpointSliceForReconcile checks whether an EndpointSlice belongs to a Service that
// belongs to a valid route.
func (r *routeReconciler) validateEndpointSliceForReconcile(esl *discoveryv1.EndpointSlice) bool {
	// find the Service corresponding to this EndpointSlice
	// TODO: also check ownership
	svcName, ok := esl.GetLabels()[discoveryv1.LabelServiceName]
	if !ok {
		r.log.Info("EndpointSlice has no service-name label", "endpointslice", store.GetObjectKey(esl))
		return false
	}

	svc := &corev1.Service{}
	if err := r.Get(context.Background(), types.NamespacedName{
		Namespace: esl.GetNamespace(),
		Name:      svcName,
	}, svc); err != nil {
		// not fatal
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "Error getting Service for EndpointSlice",
				"namespace", esl.GetNamespace(),
				"name", svcName)
		}
		return false
	}

	return r.validateBackendServiceForReconcile(svc)
}

// getServiceForBackend finds the Service associated with a backendRef
func (r *routeReconciler) getServiceForBackend(ctx context.Context, ro client.Object, ref *stnrgwv1.BackendRef) *corev1.Service {
	// if no explicit Service namespace is provided, use the route namespace to lookup the
	// Service
	namespace := ro.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	svc := corev1.Service{}
	if err := r.Get(ctx,
		types.NamespacedName{Namespace: namespace, Name: string(ref.Name)},
		&svc,
	); err != nil {
		// not fatal
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "Error getting Service", "namespace", namespace,
				"name", string(ref.Name))
			return nil
		}

		r.log.Info("No Service found for route backend", "route",
			store.GetObjectKey(ro), "namespace", namespace,
			"name", string(ref.Name))
		return nil
	}

	return &svc
}

// getEndpointSlicesForBackend finds all EndpointSlices associated with a backendRef
func (r *routeReconciler) getEndpointSlicesForBackend(ctx context.Context, ro client.Object, ref *stnrgwv1.BackendRef) []client.Object {
	// if no explicit namespace is provided, look up the EndpointSlices in the route namespace
	namespace := ro.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	// find the EndpointSlice corresponding to the backend service
	esls := discoveryv1.EndpointSliceList{}
	labelSelector := labels.SelectorFromSet(labels.Set{discoveryv1.LabelServiceName: string(ref.Name)})
	listOptions := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	if err := r.List(ctx, &esls, listOptions); err != nil {
		r.log.Error(err, "Error getting EndpointSlices for backend service",
			"namespace", namespace, "backend-name", string(ref.Name))
		return []client.Object{}
	}

	es := make([]client.Object, len(esls.Items))
	for i := range esls.Items {
		es[i] = &esls.Items[i]
	}

	if len(es) == 0 {
		r.log.Info("No EndpointSlice found for backend", "route",
			store.GetObjectKey(ro), "backend-ref",
			store.DumpBackendRef(ref))
	}

	return es
}

// getStaticServiceForBackend finds the StaticService associated with a backendRef
func (r *routeReconciler) getStaticServiceForBackend(ctx context.Context, ro client.Object, ref *stnrgwv1.BackendRef) *stnrgwv1.StaticService {
	svc := stnrgwv1.StaticService{}

	// if no explicit StaticService namespace is provided, use the route namespace to lookup
	// the StaticService
	namespace := ro.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	if err := r.Get(ctx,
		types.NamespacedName{Namespace: namespace, Name: string(ref.Name)},
		&svc,
	); err != nil {
		// not fatal
		if !apierrors.IsNotFound(err) {
			r.log.Error(err, "Error getting StaticService", "namespace", namespace,
				"name", string(ref.Name))
			return nil
		}

		r.log.Info("No StaticService found for route backend", "route",
			store.GetObjectKey(ro), "namespace", namespace,
			"name", string(ref.Name))
		return nil
	}

	return &svc
}

// gwAPIRouteVersion returns the Gateway API version at which the cluster serves an official
// route resource, preferring the graduated v1 version over the deprecated v1alpha2.
func (r *routeReconciler) gwAPIRouteVersion(mgr manager.Manager, v1Obj, v1a2Obj client.Object, resourceName string) (string, error) {
	served, err := r.isRouteResourceServed(mgr, v1Obj, resourceName)
	if err != nil {
		return config.GwAPIVersionUnavailable, err
	}
	if served {
		return config.GwAPIVersionV1, nil
	}

	served, err = r.isRouteResourceServed(mgr, v1a2Obj, resourceName)
	if err != nil {
		return config.GwAPIVersionUnavailable, err
	}
	if served {
		return config.GwAPIVersionV1A2, nil
	}

	return config.GwAPIVersionUnavailable, nil
}

// isRouteResourceServed checks whether the API server serves the given route resource at the
// group/version of the object.
func (r *routeReconciler) isRouteResourceServed(mgr manager.Manager, obj client.Object, resourceName string) (bool, error) {
	// Build a discovery client
	d, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return false, fmt.Errorf("failed to obtain a discovery client: %w", err)
	}

	// Get the Groupversion
	gvk, err := apiutil.GVKForObject(obj, mgr.GetScheme())
	if err != nil {
		return false, fmt.Errorf("failed to get GVK for %T: %w", obj, err)
	}
	gvStr := gvk.GroupVersion().String()

	resList, err := d.ServerResourcesForGroupVersion(gvStr)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get server resources for %s: %w", gvStr, err)
	}

	for _, r := range resList.APIResources {
		if r.Name == resourceName {
			return true, nil
		}
	}

	return false, nil
}

// canonicalRoute converts any supported route object into the canonical STUNner-native
// representation for indexing. Returns nil for unsupported objects.
func canonicalRoute(o client.Object) (client.Object, []stnrgwv1.RouteRule) {
	switch ro := o.(type) {
	case *stnrgwv1.UDPRoute:
		return ro, ro.Spec.Rules
	case *gwapiv1.UDPRoute:
		c := stnrgwv1.ConvertV1UDPRouteToStnrV1(ro)
		return c, c.Spec.Rules
	case *gwapiv1a2.UDPRoute:
		c := stnrgwv1.ConvertV1A2UDPRouteToStnrV1(ro)
		return c, c.Spec.Rules
	case *stnrgwv1.TCPRoute:
		return ro, ro.Spec.Rules
	case *gwapiv1.TCPRoute:
		c := stnrgwv1.ConvertV1TCPRouteToStnrV1(ro)
		return c, c.Spec.Rules
	case *gwapiv1a2.TCPRoute:
		c := stnrgwv1.ConvertV1A2TCPRouteToStnrV1(ro)
		return c, c.Spec.Rules
	case *stnrgwv1a1.TURNRoute:
		return ro, ro.Spec.Rules
	default:
		return nil, nil
	}
}

// secretTURNRouteIndexFunc indexes a TURNRoute by the Secret its tunnel authenticates with.
func secretTURNRouteIndexFunc(o client.Object) []string {
	ro, ok := o.(*stnrgwv1a1.TURNRoute)
	if !ok {
		return []string{}
	}
	n, ok := tunnelAuthSecretName(ro)
	if !ok {
		return []string{}
	}
	return []string{n.String()}
}

// tunnelAuthSecretName returns the name of the Secret the tunnel of a TURNRoute authenticates
// with; ok is false when the route references no Secret.
func tunnelAuthSecretName(ro *stnrgwv1a1.TURNRoute) (types.NamespacedName, bool) {
	ref := ro.Spec.Tunnel.AuthRef
	if ref == nil || (ref.Group != nil && *ref.Group != corev1.GroupName && *ref.Group != "v1") ||
		(ref.Kind != nil && *ref.Kind != "Secret") {
		return types.NamespacedName{}, false
	}
	namespace := ro.GetNamespace()
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}
	return types.NamespacedName{Namespace: namespace, Name: string(ref.Name)}, true
}

func serviceRouteIndexFunc(o client.Object) []string {
	ro, rules := canonicalRoute(o)
	if ro == nil {
		return []string{}
	}

	var services []string
	for _, rule := range rules {
		for _, backend := range rule.BackendRefs {
			if !store.IsReferenceService(&backend) {
				continue
			}

			if backend.Kind == nil || string(*backend.Kind) == "Service" {
				// if no explicit Service namespace is provided, use the route
				// namespace to lookup the provided Service
				namespace := ro.GetNamespace()
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

func staticServiceRouteIndexFunc(o client.Object) []string {
	ro, rules := canonicalRoute(o)
	if ro == nil {
		return []string{}
	}

	var staticServices []string
	for _, rule := range rules {
		for _, backend := range rule.BackendRefs {
			backend := backend

			if !store.IsReferenceStaticService(&backend) {
				continue
			}

			// if no explicit StaticService namespace is provided, use the route
			// namespace to lookup the provided static service
			namespace := ro.GetNamespace()
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

func (r *routeReconciler) Name() ControllerName { return RouteControllerName }

// ServiceLabelSelectorPredicate is the typed version of LabelSelectorPredicate that somehow seems
// to be missing in controller-runtime to construct a TypedPredicate from a LabelSelector.  Only
// objects matching the LabelSelector will be admitted.
func ServiceLabelSelectorPredicate(s metav1.LabelSelector) (predicate.TypedPredicate[*corev1.Service], error) {
	selector, err := metav1.LabelSelectorAsSelector(&s)
	if err != nil {
		return predicate.TypedFuncs[*corev1.Service]{}, err
	}
	return predicate.NewTypedPredicateFuncs[*corev1.Service](func(o *corev1.Service) bool {
		return selector.Matches(labels.Set(o.GetLabels()))
	}), nil
}
