// This file contains code derived from Envoy Gateway,
// https://github.com/envoyproxy/gateway
// and is provided here subject to the following:
// Copyright Envoy Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	// "errors"
	// "fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/resource"
)

const (
	secretGatewayIndex = "secretGatewayIndex"
)

type gatewayReconciler struct {
	client.Client
	resources *resource.Store
	log       logr.Logger
}

func RegisterGatewayController(mgr manager.Manager, resources *resource.Store, log logr.Logger) error {
	r := &gatewayReconciler{
		Client:    mgr.GetClient(),
		resources: resources,
		log:       log,
	}

	c, err := controller.New("gateway", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created gateway controller")

	// Only enqueue GatewayClass objects that match this controller name.
	if err := c.Watch(
		&source.Kind{Type: &gwapiv1b1.Gateway{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.hasMatchingController),
	); err != nil {
		return err
	}
	r.log.Info("watching gateway objects")

	// Create an index on Secrets: this will be used later for creating reconcile requests for
	// the Gateways affected by a change in a Secret.
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Secret{}, secretGatewayIndex,
		func(rawObj client.Object) []string {
			// Extract the ConfigMap name from the ConfigDeployment Spec, if one is provided
			gw := rawObj.(*gwapiv1b1.Gateway)
			var refSecrets []string

			if !r.hasMatchingController(gw) {
				return []string{}
			}

			for j := range gw.Spec.Listeners {
				l := gw.Spec.Listeners[j]

				if !terminatesTLS(&l) {
					continue
				}

				for i := range l.TLS.CertificateRefs {
					r := l.TLS.CertificateRefs[i]
					if !refsSecret(&r) {
						continue
					}

					// If an explicit Secret namespace is not provided, use the
					// Gateway namespace.
					refSecrets = append(refSecrets,
						types.NamespacedName{
							Namespace: resource.NamespaceDerefOr(r.Namespace,
								gw.GetNamespace()),
							Name: string(r.Name),
						}.String(),
					)
				}
			}
			return refSecrets
		}); err != nil {
		return err
	}

	// Trigger gateway reconciliation when a Secret that is referenced by a managed Gateway has
	// changed.
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}},
		handler.EnqueueRequestsFromMapFunc(r.findSecretsForGateway)); err != nil {
		return err
	}
	return nil
}

func (r *gatewayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("gateway-class", req.Name)
	log.Info("reconciling")

	var gw gwapiv1b1.Gateway
	found := true

	err := r.Get(ctx, req.NamespacedName, &gw)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get Gateway")
			return reconcile.Result{}, err
		}
		found = false
	}

	if !found {
		r.resources.Gateways.Delete(req)
		return reconcile.Result{}, nil
	}

	r.resources.Gateways.Store(req, &gw)

	return reconcile.Result{}, nil
}

// hasMatchingController returns true if the provided object is a Gateway using a GatewayClass
// matching the configured gatewayclass controller name.
func (r *gatewayReconciler) hasMatchingController(obj client.Object) bool {
	gw, ok := obj.(*gwapiv1b1.Gateway)
	if !ok {
		r.log.Info("unexpected object type, bypassing reconciliation", "object", obj)
		return false
	}

	gc := &gwapiv1b1.GatewayClass{}
	key := types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}
	if err := r.Get(context.Background(), key, gc); err != nil {
		r.log.Info("no matching gatewayclass", "name", gw.Spec.GatewayClassName)
		return false
	}

	if string(gc.Spec.ControllerName) == config.ControllerName {
		return false
	}

	return true
}

// findSecretsForGateway finds the secrets that are referenced in one of the gateways and creates a
// reconcile request for the gateway objects identified.
func (r *gatewayReconciler) findSecretsForGateway(secret client.Object) []reconcile.Request {
	gatewaysForSecret := &gwapiv1b1.GatewayList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(secretGatewayIndex, secret.GetName()),
		Namespace:     secret.GetNamespace(),
	}

	err := r.List(context.TODO(), gatewaysForSecret, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(gatewaysForSecret.Items))
	for i := range gatewaysForSecret.Items {
		item := gatewaysForSecret.Items[i]
		requests[i] = reconcile.Request{resource.NamespacedName(&item)}
	}
	return requests
}

// terminatesTLS returns true if the provided gateway contains a listener configured for TLS
// termination.
func terminatesTLS(listener *gwapiv1b1.Listener) bool {
	if listener.TLS != nil && (listener.TLS.Mode == nil || *listener.TLS.Mode == gwapiv1b1.TLSModeTerminate) {
		return true
	}
	return false
}

// refsSecret returns true if ref refers to a Secret.
func refsSecret(ref *gwapiv1b1.SecretObjectReference) bool {
	return (ref.Group == nil || *ref.Group == corev1.GroupName) &&
		(ref.Kind == nil || *ref.Kind == resource.KindSecret)
}
