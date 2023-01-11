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
	// "fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// GatewayConfigReconciler reconciles a GatewayConfig object
type gatewayConfigReconciler struct {
	client.Client
	eventCh chan event.Event
	log     logr.Logger
}

func RegisterGatewayConfigController(mgr manager.Manager, ch chan event.Event, log logr.Logger) error {
	r := &gatewayConfigReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("gatewayconfig-controller"),
	}

	c, err := controller.New("gatewayconfig", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created gatewayconfig controller")

	if err := c.Watch(
		&source.Kind{Type: &stnrv1a1.GatewayConfig{}},
		&handler.EnqueueRequestForObject{},
		// trigger when the GatewayConfig spec changes
		predicate.GenerationChangedPredicate{},
	); err != nil {
		return err
	}
	r.log.Info("watching gatewayconfig objects")

	return nil
}

func (r *gatewayConfigReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("gateway-config", req.String())
	log.Info("reconciling")

	var gc stnrv1a1.GatewayConfig
	found := true

	err := r.Get(ctx, req.NamespacedName, &gc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get GatewayConfig")
			return reconcile.Result{}, err
		}
		found = false
	}

	if !found {
		store.GatewayConfigs.Remove(req.NamespacedName)
		r.eventCh <- event.NewEventRender()
		return reconcile.Result{}, nil
	}

	store.GatewayConfigs.Upsert(&gc)
	r.eventCh <- event.NewEventRender()

	return reconcile.Result{}, nil
}
