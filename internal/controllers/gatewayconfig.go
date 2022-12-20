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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/resource"
)

// GatewayConfigReconciler reconciles a GatewayConfig object
type gatewayConfigReconciler struct {
	client.Client
	resources *resource.Store
	log       logr.Logger
}

func RegisterGatewayConfigController(mgr manager.Manager, resources *resource.Store, log logr.Logger) error {
	r := &gatewayConfigReconciler{
		Client:    mgr.GetClient(),
		resources: resources,
		log:       log,
	}

	c, err := controller.New("gatewayconfig", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created gatewayconfig controller")

	// Only enqueue GatewayClass objects that match this controller name.
	if err := c.Watch(
		&source.Kind{Type: &stnrv1a1.GatewayConfig{}},
		&handler.EnqueueRequestForObject{},
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
		r.resources.GatewayConfigs.Delete(req)
		return reconcile.Result{}, nil
	}

	r.resources.GatewayConfigs.Store(req, &gc)

	return reconcile.Result{}, nil
}
