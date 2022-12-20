package controllers

import (
	"context"
	// "errors"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/resource"
)

type gatewayClassReconciler struct {
	client.Client
	resources *resource.Store
	log       logr.Logger
}

func RegisterGatewayClassController(mgr manager.Manager, resources *resource.Store, log logr.Logger) error {
	r := &gatewayClassReconciler{
		Client:    mgr.GetClient(),
		resources: resources,
		log:       log,
	}

	c, err := controller.New("gatewayclass", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created gatewayclass controller")

	// Only enqueue GatewayClass objects that match this controller name.
	if err := c.Watch(
		&source.Kind{Type: &gwapiv1b1.GatewayClass{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return err
	}
	r.log.Info("watching gatewayclass objects")

	return nil
}

func (r *gatewayClassReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("gateway-class", req.Name)
	log.Info("reconciling")

	var gc gwapiv1b1.GatewayClass
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get GatewayClass")
			return reconcile.Result{}, err
		}
		found = false
	}

	// do we manage this gatewayclass?
	if string(gc.Spec.ControllerName) != config.ControllerName {
		log.Info("ignoring GatewayClass for an unknown controller", "controller-name",
			string(gc.Spec.ControllerName), "expecting", config.ControllerName)
		return reconcile.Result{}, nil
	}

	if err = validateGatewayClass(&gc); err != nil {
		log.Error(err, "invalid GatewayClass", "gateway-class", fmt.Sprintf("%#v", gc))
		return reconcile.Result{}, fmt.Errorf("invalid GatewayClass: %w", err)
	}

	if !found {
		r.resources.GatewayClasses.Delete(req.Name)
		return reconcile.Result{}, nil
	}

	r.resources.GatewayClasses.Store(gc.GetName(), &gc)

	return reconcile.Result{}, nil
}

// hasMatchingController returns true if the provided object is a GatewayClass with a
// Spec.Controller string matching the controller string, or false otherwise.
func (r *gatewayClassReconciler) hasMatchingController(obj client.Object) bool {
	gc, ok := obj.(*gwapiv1b1.GatewayClass)
	if !ok {
		return false
	}

	if string(gc.Spec.ControllerName) == config.ControllerName {
		return true
	}

	return false
}

// must have a ParametersReference, ref must point to a GatewayConfig, namespace in the ref must be
// set (GatewayConfigs are namespaced)
func validateGatewayClass(gc *gwapiv1b1.GatewayClass) error {
	ref := gc.Spec.ParametersRef
	if ref == nil {
		return fmt.Errorf("empty ParametersRef in GatewayClassSpec: %#v", gc.Spec)
	}

	if string(ref.Group) != stnrv1a1.GroupVersion.Group {
		return fmt.Errorf("invalid group in ParametersRef %q, expecting %q",
			string(ref.Group), stnrv1a1.GroupVersion.Group)
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
