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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// -----------------------------------------------------------------------------
// GatewayClassReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch

type gatewayClassReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	eventCh chan event.Event
}

func RegisterGatewayClassController(mgr manager.Manager, ch chan event.Event) error {
	r := &gatewayClassReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha2.GatewayClass{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("gateway-class", req.Name)
	log.Info("Reconciling GatewayClass")

	var gc gatewayv1alpha2.GatewayClass
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get GatewayClass")
			return reconcile.Result{}, err
		}
		found = false
	}

	// do we manage this gatewayclass?
	if string(gc.Spec.ControllerName) != config.ControllerName {
		log.V(1).Info("ignoring GatewayClass for an unknown controller", "controller-name",
			string(gc.Spec.ControllerName), "expecting", config.ControllerName)
		return reconcile.Result{}, nil
	}

	if err = validateGatewayClass(&gc); err != nil {
		log.Error(err, "invalid GatewayClass", "gateway-class", fmt.Sprintf("%#v", gc))
		return reconcile.Result{}, fmt.Errorf("invalid GatewayClass: %w", err)
	}

	if !found {
		r.eventCh <- event.NewEventDelete(event.EventKindGatewayClass, req.NamespacedName)
		return reconcile.Result{}, nil
	}

	r.eventCh <- event.NewEventUpsert(&gc)

	return reconcile.Result{}, nil
}

// must have a ParametersReference, ref must point to a GatewayConfig, namespace in the ref must be
// set (GatewayConfigs are namespaced)
func validateGatewayClass(gc *gatewayv1alpha2.GatewayClass) error {
	ref := gc.Spec.ParametersRef
	if ref == nil {
		return fmt.Errorf("empty ParametersRef in GatewayClassSpec: %#v", gc.Spec)
	}

	if string(ref.Group) != stunnerv1alpha1.GroupVersion.Group {
		return fmt.Errorf("invalid group in ParametersRef %q, expecting %q",
			string(ref.Group), stunnerv1alpha1.GroupVersion.Group)
	}

	if string(ref.Kind) != "GatewayConfig" {
		return fmt.Errorf("invalid Kind in ParametersRef %q, expecting %q",
			string(ref.Kind), "GatewayConfig")
	}

	if ref.Namespace == nil {
		return fmt.Errorf("invalid Namespace in ParametersRef: namespace must be set")
	}

	return nil
}
