package controllers

import (
	"context"
	// "errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// -----------------------------------------------------------------------------
// UDPRouteReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=udproutes/finalizers,verbs=update

type udpRouteReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	store   store.Store
	eventCh chan event.Event
}

func RegisterUDPRouteController(mgr manager.Manager, store store.Store, ch chan event.Event) error {
	r := &udpRouteReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		store:   store,
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha2.UDPRoute{}).
		Complete(r)
}

func (r *udpRouteReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("udproute", req.Name)
	log.V(2).Info("Reconciling UDPRoute", "request", req)

	var gc gatewayv1alpha2.UDPRoute
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get UDPRoute")
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
	e.Origin = "UDPRoute"
	e.Reason = fmt.Sprintf("update on %q", req.NamespacedName)

	r.eventCh <- e

	return reconcile.Result{}, nil
}
