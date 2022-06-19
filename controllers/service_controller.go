package controllers

import (
	"context"
	// "errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// -----------------------------------------------------------------------------
// ServiceReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

//+kubebuilder:rbac:groups=core,resources=services,verbs=create;get;list;watch;update;patch
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get

type serviceReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	store   store.Store
	eventCh chan event.Event
}

func RegisterServiceController(mgr manager.Manager, store store.Store, ch chan event.Event) error {
	r := &serviceReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		store:   store,
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Complete(r)
}

func (r *serviceReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("service", req.Name)
	log.V(2).Info("Reconciling Service", "request", req)

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
		r.store.Remove(req.NamespacedName)
	}

	r.store.Upsert(&gc)

	// trigger a config render for this namespace
	e := event.NewEvent(event.EventTypeRender)
	e.Origin = "Service"
	e.Reason = fmt.Sprintf("update on %q", req.NamespacedName)

	r.eventCh <- e

	return reconcile.Result{}, nil
}
