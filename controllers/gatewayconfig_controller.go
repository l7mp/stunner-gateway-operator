/*
Copyright 2022 The l7mp/stunner team.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// -----------------------------------------------------------------------------
// GatewayConfigReconciler - RBAC Permissions
// -----------------------------------------------------------------------------

//+kubebuilder:rbac:groups=stunner.l7mp.io,resources=gatewayconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stunner.l7mp.io,resources=gatewayconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=stunner.l7mp.io,resources=gatewayconfigs/finalizers,verbs=update

// GatewayConfigReconciler reconciles a GatewayConfig object
type gatewayConfigReconciler struct {
	client.Client
	scheme  *runtime.Scheme
	store   store.Store
	eventCh chan event.Event
}

func RegisterGatewayConfigController(mgr manager.Manager, store store.Store, ch chan event.Event) error {
	r := &gatewayConfigReconciler{
		Client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		store:   store,
		eventCh: ch,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&stunnerv1alpha1.GatewayConfig{}).
		Complete(r)
}

func (r *gatewayConfigReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := log.FromContext(ctx).WithValues("gateway-config", req.Name)
	log.V(1).Info("Reconciling GatewayConfig", "request", req)

	var gc stunnerv1alpha1.GatewayConfig
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get GatewayConfig")
			return reconcile.Result{}, err
		}
		found = false
	}

	if !found {
		r.store.Remove(req.NamespacedName)
	}

	r.store.Upsert(&gc)

	// trigger a config render for this namespace
	e := event.NewEventRender()
	e.Origin = "GatewayConfig"
	e.Reason = fmt.Sprintf("update on %q", req.NamespacedName)
	r.eventCh <- e

	return reconcile.Result{}, nil
}
