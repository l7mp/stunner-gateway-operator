package controllers

import (
	"context"
	// "errors"
	// "fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlevent "sigs.k8s.io/controller-runtime/pkg/event"
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

type endpointReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	eventCh chan event.Event
}

// filter updates unless the endpoint address list has changed
func endpointsChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e ctrlevent.UpdateEvent) bool {
			return endpointNum(e.ObjectOld) != endpointNum(e.ObjectNew)
		},
	}
}

func RegisterEndpointController(mgr manager.Manager, ch chan event.Event) error {
	r := &endpointReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Endpoints{}).
		WithEventFilter(endpointsChangedPredicate()).
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

	log.V(1).Info("Endpoint upsert", "endpoint-num", endpointNum(&gc))

	r.eventCh <- event.NewEventUpsert(&gc)

	return reconcile.Result{}, nil
}

func endpointNum(o client.Object) int {
	e, ok := o.(*corev1.Endpoints)
	if !ok {
		return 0
	}

	ret := 0
	for _, s := range e.Subsets {
		ret += len(s.Addresses) + len(s.NotReadyAddresses)
	}

	return ret
}
