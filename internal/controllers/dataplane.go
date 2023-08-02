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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// DataplaneReconciler reconciles a Dataplane object.
type dataplaneReconciler struct {
	client.Client
	eventCh chan event.Event
	log     logr.Logger
}

func RegisterDataplaneController(mgr manager.Manager, ch chan event.Event, log logr.Logger) error {
	r := &dataplaneReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("dataplane-controller"),
	}

	c, err := controller.New("dataplane", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created dataplane controller")

	if err := c.Watch(
		// &source.Kind{Type: &stnrv1a1.Dataplane{}},
		source.Kind(mgr.GetCache(), &stnrv1a1.Dataplane{}),
		&handler.EnqueueRequestForObject{},
		// trigger when the Dataplane spec changes
		predicate.GenerationChangedPredicate{},
	); err != nil {
		return err
	}
	r.log.Info("watching dataplane objects")

	return nil
}

func (r *dataplaneReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("dataplane", req.String())
	log.Info("reconciling")

	dataplaneList := []client.Object{}

	// find all Dataplanes
	dpList := &stnrv1a1.DataplaneList{}
	if err := r.List(ctx, dpList); err != nil {
		r.log.Info("no dataplane resource found")
		return reconcile.Result{}, err
	}

	for _, dp := range dpList.Items {
		dp := dp
		r.log.V(1).Info("processing Dataplane")

		dataplaneList = append(dataplaneList, &dp)
	}

	store.Dataplanes.Reset(dataplaneList)
	r.log.V(2).Info("reset Dataplane store", "configs", store.Dataplanes.String())

	r.eventCh <- event.NewEventRender()

	return reconcile.Result{}, nil
}
