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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// -----------------------------------------------------------------------------
// NodeReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

// +kubebuilder:rbac:groups=,resources=nodes,verbs=get;list;watch

type nodeReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	eventCh chan event.Event
}

func RegisterNodeController(mgr manager.Manager, ch chan event.Event) error {
	r := &nodeReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}

func (r *nodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("node", req.Name)
	log.Info("Reconciling Node")

	var gc corev1.Node
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get Node")
			return reconcile.Result{}, err
		}
		found = false
	}

	if !found {
		r.eventCh <- event.NewEventDelete(event.EventKindNode, req.NamespacedName)
		return reconcile.Result{}, nil
	}

	r.eventCh <- event.NewEventUpsert(&gc)

	return reconcile.Result{}, nil
}
