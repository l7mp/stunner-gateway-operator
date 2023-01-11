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

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

const (
	secretGatewayIndex = "secretGatewayIndex"
	classGatewayIndex  = "classGatewayIndex"
)

type gatewayReconciler struct {
	client.Client
	eventCh chan event.Event
	log     logr.Logger
}

// RegisterGatewayController registers a reconciler for Gateway and the associated Secret objects.
func RegisterGatewayController(mgr manager.Manager, ch chan event.Event, log logr.Logger) error {
	ctx := context.Background()
	r := &gatewayReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("gateway-controller"),
	}

	c, err := controller.New("gateway", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created gateway controller")

	// watch Gateway objects that match the controller name
	if err := c.Watch(
		&source.Kind{Type: &gwapiv1a2.Gateway{}},
		&handler.EnqueueRequestForObject{},
		//trigger when the Spec or an annotation changes on a Gateway we manage
		predicate.And(
			predicate.Or(
				predicate.GenerationChangedPredicate{},
				predicate.AnnotationChangedPredicate{},
			),
			predicate.NewPredicateFuncs(r.validateGatewayForReconcile),
		),
	); err != nil {
		return err
	}
	r.log.Info("watching gateway objects")

	// index Gateway objects as per the referenced Secrets
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.Gateway{}, secretGatewayIndex,
		secretGatewayIndexFunc); err != nil {
		return err
	}

	// index Gateway objects as per the referenced GatewayClass
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1a2.Gateway{}, classGatewayIndex,
		classGatewayIndexFunc); err != nil {
		return err
	}

	// watch Secret objects referenced by one of our Gateways
	if err := c.Watch(
		&source.Kind{Type: &corev1.Secret{}},
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.validateSecretForReconcile),
	); err != nil {
		return err
	}
	r.log.Info("watching secret objects")

	return nil
}

// Reconcile handles updates to a Gateway managed by this controller or a Secret referenced by one
// of the Gateways managed by this controller.
func (r *gatewayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("object", req.String())
	log.Info("reconciling")

	gatewayList := []client.Object{}
	secretList := []client.Object{}

	// find Gateways managed by this controller
	gwClasses := &gwapiv1a2.GatewayClassList{}
	if err := r.List(ctx, gwClasses); err != nil {
		r.log.Error(err, "error obtaining  GatewayClasses", "name", config.ControllerName)
		return reconcile.Result{}, err
	}

	for _, gc := range gwClasses.Items {
		gc := gc
		// do we manage this class
		if string(gc.Spec.ControllerName) != config.ControllerName {
			continue
		}

		// get gateways for this class
		gateways := &gwapiv1a2.GatewayList{}
		if err := r.List(ctx, gateways, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(classGatewayIndex, gc.GetName()),
		}); err != nil {
			r.log.Info("no associated Gateways found for GatewayClass", "name", config.ControllerName)
			return reconcile.Result{}, err
		}

		for _, gw := range gateways.Items {
			gw := gw
			r.log.V(1).Info("processing Gateway", "namespace", gw.Namespace, "name", gw.Name)

			gatewayList = append(gatewayList, &gw)

			for _, listener := range gw.Spec.Listeners {
				if listener.TLS == nil ||
					(listener.TLS.Mode != nil && *listener.TLS.Mode != gwapiv1a2.TLSModeTerminate) ||
					(string(listener.Protocol) != "TLS" && string(listener.Protocol) != "DTLS") {
					continue
				}
				for _, ref := range listener.TLS.CertificateRefs {
					if (ref.Group != nil && *ref.Group != corev1.GroupName) ||
						(ref.Kind != nil && *ref.Kind != "Secret") {
						continue
					}

					// obtain ref'd secret
					secret := corev1.Secret{}

					// if no explicit Service namespace is provided, use the UDPRoute
					// namespace to lookup the provided Service
					secretNamespace := gw.Namespace
					if ref.Namespace != nil {
						secretNamespace = string(*ref.Namespace)
					}

					if err := r.Get(ctx,
						types.NamespacedName{Namespace: secretNamespace, Name: string(ref.Name)},
						&secret,
					); err != nil {
						// not fatal
						if !apierrors.IsNotFound(err) {
							r.log.Error(err, "error getting Secret", "namespace",
								secretNamespace, "name", string(ref.Name))
							continue
						}

						r.log.Info("no Secret found for Gateway", "gateway",
							store.GetObjectKey(&gw), "listener", listener.Name,
							"namespace", secretNamespace, "name", string(ref.Name))
						continue
					}

					// TODO: check for ReferenceGrants

					secretList = append(secretList, &secret)
				}
			}
		}
	}

	store.Gateways.Reset(gatewayList)
	r.log.V(2).Info("reset Gateway store", "gateways", store.Gateways.String())

	store.Secrets.Reset(secretList)
	r.log.V(2).Info("reset Secret store", "secrets", store.Secrets.String())

	r.eventCh <- event.NewEventRender()

	return reconcile.Result{}, nil
}

// validateGatewayForReconcile returns true if the provided object is a Gateway using a
// GatewayClass matching the configured gatewayclass controller name.
func (r *gatewayReconciler) validateGatewayForReconcile(o client.Object) bool {
	gw := o.(*gwapiv1a2.Gateway)
	gc := &gwapiv1a2.GatewayClass{}
	key := types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}
	if err := r.Get(context.Background(), key, gc); err != nil {
		r.log.Info("no matching gatewayclass", "name", gw.Spec.GatewayClassName)
		return false
	}

	if string(gc.Spec.ControllerName) == config.ControllerName {
		return true
	}

	return false
}

// validateSecretForReconcile checks whether the Secret belongs to a valid Gateway.
func (r *gatewayReconciler) validateSecretForReconcile(obj client.Object) bool {
	secret := obj.(*corev1.Secret)
	gwList := &gwapiv1a2.GatewayList{}
	secretName := store.GetNamespacedName(secret).String()
	if err := r.List(context.Background(), gwList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(secretGatewayIndex, secretName),
	}); err != nil {
		r.log.Error(err, "unable to find associated gateways", "secret", secretName)
		return false
	}

	if len(gwList.Items) == 0 {
		return false
	}

	for _, gw := range gwList.Items {
		gw := gw
		if r.validateGatewayForReconcile(&gw) {
			return true
		}
	}

	return false
}

func secretGatewayIndexFunc(o client.Object) []string {
	gateway := o.(*gwapiv1a2.Gateway)
	var secretReferences []string

	for _, listener := range gateway.Spec.Listeners {
		if listener.TLS == nil || (listener.TLS.Mode != nil && *listener.TLS.Mode != gwapiv1a2.TLSModeTerminate) {
			continue
		}
		for _, cert := range listener.TLS.CertificateRefs {
			if cert.Kind == nil || (cert.Kind != nil && string(*cert.Kind) == "Secret") {
				// if no explicit Secret namespace is provided, use the Gateway
				// namespace to lookup the provided Secret Name
				namespace := gateway.Namespace
				if cert.Namespace != nil {
					namespace = string(*cert.Namespace)
				}
				secretReferences = append(secretReferences,
					types.NamespacedName{
						Namespace: namespace,
						Name:      string(cert.Name),
					}.String(),
				)
			}
		}
	}

	return secretReferences
}

func classGatewayIndexFunc(o client.Object) []string {
	gateway := o.(*gwapiv1a2.Gateway)
	return []string{string(gateway.Spec.GatewayClassName)}
}
