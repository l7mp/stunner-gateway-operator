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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// -----------------------------------------------------------------------------
// EndpointReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

//+kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=endpoints/status,verbs=get

type endpointReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	eventCh chan event.Event
}

func RegisterEndpointController(mgr manager.Manager, ch chan event.Event) error {
	r := &endpointReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Endpoints{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (r *endpointReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("endpoints", req.Name)
	log.Info("Reconciling Endpoints")

	var gc corev1.Endpoints
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get Endpoints")
			return reconcile.Result{}, err
		}
		found = false
	}

	if !found {
		r.eventCh <- event.NewEventDelete(event.EventKindEndpoint, req.NamespacedName)
		return reconcile.Result{}, nil
	}

	r.eventCh <- event.NewEventUpsert(&gc)

	return reconcile.Result{}, nil
}
