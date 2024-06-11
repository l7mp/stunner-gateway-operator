// This file contains code derived from Envoy Gateway,
// https://github.com/envoyproxy/gateway
// and is provided here subject to the following:
// Copyright Envoy Gateway Authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
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

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

const (
	secretGatewayIndex = "secretGatewayIndex"
	classGatewayIndex  = "classGatewayIndex"
)

type gatewayReconciler struct {
	client.Client
	eventCh     chan event.Event
	terminating bool
	log         logr.Logger
}

// NewGatewayController registers a reconciler for Gateway and the associated Secret objects.
func NewGatewayController(mgr manager.Manager, ch chan event.Event, log logr.Logger) (Controller, error) {
	ctx := context.Background()
	r := &gatewayReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("gateway-controller"),
	}

	c, err := controller.New("gateway", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	r.log.Info("Created Gateway controller")

	// watch GatewayClass objects that match this controller name
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &gwapiv1.GatewayClass{}),
		&handler.EnqueueRequestForObject{},
		// trigger when the spec changes on a GatewayClass we manage
		predicate.And(
			predicate.NewPredicateFuncs(r.hasMatchingController),
			predicate.GenerationChangedPredicate{},
		),
	); err != nil {
		return nil, err
	}
	r.log.Info("Watching GatewayClass objects")

	// watch Gateway objects that match the controller name
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &gwapiv1.Gateway{}),
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
		return nil, err
	}
	r.log.Info("Watching Gateway objects")

	// index Gateway objects as per the referenced Secrets
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1.Gateway{}, secretGatewayIndex,
		secretGatewayIndexFunc); err != nil {
		return nil, err
	}

	// index Gateway objects as per the referenced GatewayClass
	if err := mgr.GetFieldIndexer().IndexField(ctx, &gwapiv1.Gateway{}, classGatewayIndex,
		classGatewayIndexFunc); err != nil {
		return nil, err
	}

	// watch Secret objects referenced by one of our Gateways
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &corev1.Secret{}),
		&handler.EnqueueRequestForObject{},
		predicate.NewPredicateFuncs(r.validateSecretForReconcile),
	); err != nil {
		return nil, err
	}
	r.log.Info("watching Secret objects")

	if config.DataplaneMode == config.DataplaneModeManaged {
		// watch Deployment objects referenced by one of our Gateways
		if err := c.Watch(
			source.Kind(mgr.GetCache(), &appv1.Deployment{}),
			&handler.EnqueueRequestForObject{},
			predicate.NewPredicateFuncs(r.validateDeploymentForReconcile),
		); err != nil {
			return nil, err
		}
		r.log.Info("Watching Deployment objects")
	}

	// NOTE: LoadBalancer Service resources are watched by the UDPRoute controller (together
	// with backend Services)

	return r, nil
}

// Reconcile handles updates to a Gateway managed by this controller or a Secret referenced by one
// of the Gateways managed by this controller.
func (r *gatewayReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("resource", req.String())

	if r.terminating {
		r.log.V(2).Info("Controller terminating, suppressing reconciliation")
		return reconcile.Result{}, nil
	}

	log.Info("Reconciling")
	gatewayClassList := []client.Object{}
	gatewayList := []client.Object{}
	secretList := []client.Object{}
	deploymentList := []client.Object{}

	// find Gateways managed by this controller
	gwClasses := &gwapiv1.GatewayClassList{}
	if err := r.List(ctx, gwClasses); err != nil {
		r.log.Error(err, "Error obtaining GatewayClasses", "name", config.ControllerName)
		return reconcile.Result{}, err
	}

	for _, gc := range gwClasses.Items {
		gc := gc
		// do we manage this class
		if string(gc.Spec.ControllerName) != config.ControllerName {
			continue
		}

		// is class valid?
		if err := validateGatewayClass(&gc); err != nil {
			r.log.Error(err, "Invalid GatewayClass", "name", store.GetObjectKey(&gc),
				"gateway-class", fmt.Sprintf("%#v", gc))
			continue
		}

		gatewayClassList = append(gatewayClassList, &gc)
		r.log.V(2).Info("Found GatewayClass", "name", store.GetObjectKey(&gc))

		// get gateways for this class
		gateways := &gwapiv1.GatewayList{}
		if err := r.List(ctx, gateways, &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(classGatewayIndex, gc.GetName()),
		}); err != nil {
			r.log.Info("No associated Gateways found for GatewayClass", "name", config.ControllerName)
			return reconcile.Result{}, err
		}

		for _, gw := range gateways.Items {
			gw := gw
			r.log.V(1).Info("Found Gateway", "namespace", gw.Namespace, "name", gw.Name)

			gatewayList = append(gatewayList, &gw)

			for _, listener := range gw.Spec.Listeners {
				if listener.TLS == nil ||
					(listener.TLS.Mode != nil && *listener.TLS.Mode != gwapiv1.TLSModeTerminate) ||
					(string(listener.Protocol) != "TLS" && string(listener.Protocol) != "DTLS" &&
						string(listener.Protocol) != "TURN-TLS" && string(listener.Protocol) != "TURN-DTLS") {
					continue
				}
				for _, ref := range listener.TLS.CertificateRefs {
					if (ref.Group != nil && *ref.Group != corev1.GroupName) ||
						(ref.Kind != nil && *ref.Kind != "Secret") {
						continue
					}

					// obtain ref'd secret
					secret := corev1.Secret{}

					// if no explicit Service namespace is provided, use the
					// Gateway namespace to lookup the provided Service
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
							r.log.Error(err, "Error getting Secret", "namespace",
								secretNamespace, "name", string(ref.Name))
							continue
						}

						r.log.Info("No Secret found for Gateway", "gateway",
							store.GetObjectKey(&gw), "listener", listener.Name,
							"namespace", secretNamespace, "name", string(ref.Name))
						continue
					}

					// TODO: check for ReferenceGrants

					r.log.V(2).Info("found Secret", "name", store.GetObjectKey(&gc))
					secretList = append(secretList, &secret)
				}
			}

			if config.DataplaneMode == config.DataplaneModeManaged {
				deploymentName := store.GetNamespacedName(&gw)

				// does the gateway actually exist?
				dp := &appv1.Deployment{}
				if err := r.Get(context.Background(), deploymentName, dp); err == nil {
					deploymentList = append(deploymentList, dp)
				}
			}
		}
	}

	store.GatewayClasses.Reset(gatewayClassList)
	r.log.V(2).Info("reset GatewayClass store", "gateway-classes",
		store.GatewayClasses.String())

	store.Gateways.Reset(gatewayList)
	r.log.V(2).Info("reset Gateway store", "gateways", store.Gateways.String())

	store.TLSSecrets.Reset(secretList)
	r.log.V(2).Info("reset Secret store", "secrets", store.TLSSecrets.String())

	store.Deployments.Reset(deploymentList)
	r.log.V(2).Info("reset Deployment store", "deployments", store.Deployments.String())

	r.eventCh <- event.NewEventReconcile()

	return reconcile.Result{}, nil
}

// hasMatchingController returns true if the provided object is a GatewayClass with a
// Spec.Controller string matching the controller string, or false otherwise.
func (r *gatewayReconciler) hasMatchingController(obj client.Object) bool {
	gc, ok := obj.(*gwapiv1.GatewayClass)
	if !ok {
		return false
	}

	if string(gc.Spec.ControllerName) == config.ControllerName {
		return true
	}

	return false
}

// validateGatewayForReconcile returns true if the provided object is a Gateway using a
// GatewayClass matching the configured gatewayclass controller name.
func (r *gatewayReconciler) validateGatewayForReconcile(o client.Object) bool {
	gw := o.(*gwapiv1.Gateway)
	gc := &gwapiv1.GatewayClass{}
	key := types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}
	if err := r.Get(context.Background(), key, gc); err != nil {
		r.log.V(1).Info("ignoring gateway: no matching gatewayclass", "gateway",
			store.GetObjectKey(gw), "name", gw.Spec.GatewayClassName)
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
	gwList := &gwapiv1.GatewayList{}
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

// validateDeploymentForReconcile checks whether there is a Gateway with the same name as the
// Deployment and the Deployment is owned by us.
func (r *gatewayReconciler) validateDeploymentForReconcile(obj client.Object) bool {
	// we don't watch Deployments in legacy mode
	if config.DataplaneMode != config.DataplaneModeManaged {
		return false
	}

	deployment := obj.(*appv1.Deployment)

	// is deployment owned by us?
	val, ok := deployment.GetLabels()[opdefault.OwnedByLabelKey]
	if !ok || val != opdefault.OwnedByLabelValue {
		return false
	}

	// gateway has the same name as the deployment
	gatewayName := store.GetNamespacedName(deployment)

	// is the related-gateway annotation set?
	val, ok = deployment.GetAnnotations()[opdefault.RelatedGatewayKey]
	if !ok || val != gatewayName.String() {
		return false
	}

	// does the gateway actually exist?
	gw := &gwapiv1.Gateway{}
	if err := r.Get(context.Background(), gatewayName, gw); err != nil {
		return false
	}

	// are we managing the gateway?
	if !r.validateGatewayForReconcile(gw) {
		return false
	}

	return true
}

// validateGatewayClass checks whether the ParametersReference ref points to an actual
// GatewayConfig and the namespace in the ref is set
func validateGatewayClass(gc *gwapiv1.GatewayClass) error {
	ref := gc.Spec.ParametersRef
	if ref == nil {
		return fmt.Errorf("Empty ParametersRef in GatewayClassSpec: %#v", gc.Spec)
	}

	if string(ref.Group) != stnrgwv1.GroupVersion.Group {
		return fmt.Errorf("Invalid group in ParametersRef %q, expecting %q",
			string(ref.Group), stnrgwv1.GroupVersion.Group)
	}

	if string(ref.Kind) != "GatewayConfig" {
		return fmt.Errorf("Invalid Kind in ParametersRef %q, expecting %q",
			string(ref.Kind), "GatewayConfig")
	}

	if ref.Namespace == nil {
		return fmt.Errorf("Invalid Namespace in ParametersRef: namespace must be set")
	}

	return nil
}

// classGatewayIndexFunc indexes Gateways on the parent GatewayClass name.
func classGatewayIndexFunc(o client.Object) []string {
	gateway := o.(*gwapiv1.Gateway)
	return []string{string(gateway.Spec.GatewayClassName)}
}

// secretGatewayIndexFunc indexes Gateways on the Secrets referred to via the TLS certRef.
func secretGatewayIndexFunc(o client.Object) []string {
	gateway := o.(*gwapiv1.Gateway)
	var secretReferences []string

	for _, listener := range gateway.Spec.Listeners {
		if listener.TLS == nil || (listener.TLS.Mode != nil && *listener.TLS.Mode != gwapiv1.TLSModeTerminate) {
			continue
		}
		for _, cert := range listener.TLS.CertificateRefs {
			if cert.Kind == nil || string(*cert.Kind) == "Secret" {

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

func (r *gatewayReconciler) Terminate() {
	r.terminating = true
}
