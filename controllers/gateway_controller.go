package controllers

import (
	"context"
	// "errors"
	// "fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate" // Required for Watching
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
)

// -----------------------------------------------------------------------------
// GatewayReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update

type gatewayReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	eventCh chan event.Event
}

func RegisterGatewayController(mgr manager.Manager, ch chan event.Event) error {
	r := &gatewayReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha2.Gateway{}).
		// don't care about status and metadata changes
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("gateway", req.Name)
	log.Info("Reconciling Gateway", "request", req)

	var gc gatewayv1alpha2.Gateway
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get Gateway")
			return reconcile.Result{}, err
		}
		found = false
	}

	if !found {
		// we don't use the "content" of gc, just the type!
		r.eventCh <- event.NewEventDelete(&gc)
		return reconcile.Result{}, nil
	}

	r.eventCh <- event.NewEventUpsert(&gc)

	return reconcile.Result{}, nil
}
