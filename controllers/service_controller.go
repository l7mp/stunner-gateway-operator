package controllers

import (
	"context"
	// "errors"
	// "fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	// "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// ctrlevent "sigs.k8s.io/controller-runtime/pkg/event"
	// "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// -----------------------------------------------------------------------------
// ServiceReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

// // // need this to learn the node IP
// // //+kubebuilder:rbac:groups=,resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=create;get;list;watch;update;patch
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get

type serviceReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	eventCh chan event.Event
}

func RegisterServiceController(mgr manager.Manager, ch chan event.Event) error {
	r := &serviceReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		// // watch only for services that expose our gateways (have a
		// // "GatewayAddressAnnotationKey" annotation)
		// WithEventFilter(predicate.NewPredicateFuncs(func(o client.Object) bool {
		// 	as := o.GetAnnotations()
		// 	_, found := as[config.GatewayAddressAnnotationKey]
		// 	return found
		// })).
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		Complete(r)
}

func (r *serviceReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("service", req.Name)
	log.Info("Reconciling Service")

	var gc corev1.Service
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get Service")
			return reconcile.Result{}, err
		}
		found = false
	}

	if !found {
		r.eventCh <- event.NewEventDelete(event.EventKindService, req.NamespacedName)
		return reconcile.Result{}, nil
	}

	r.eventCh <- event.NewEventUpsert(&gc)

	return reconcile.Result{}, nil
}
