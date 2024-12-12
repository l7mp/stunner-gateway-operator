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

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

const secretGatewayConfigIndex = "secretGatewayConfigIndex"

// GatewayConfigReconciler reconciles a GatewayConfig object
type gatewayConfigReconciler struct {
	client.Client
	eventCh     chan event.Event
	terminating bool
	log         logr.Logger
}

func NewGatewayConfigController(mgr manager.Manager, ch chan event.Event, log logr.Logger) (Controller, error) {
	ctx := context.Background()
	r := &gatewayConfigReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("gatewayconfig-controller"),
	}

	c, err := controller.New("gatewayconfig", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	r.log.Info("Created GatewayConfig controller")

	if err := c.Watch(
		source.Kind(mgr.GetCache(), &stnrgwv1.GatewayConfig{},
			&handler.TypedEnqueueRequestForObject[*stnrgwv1.GatewayConfig]{},
			predicate.TypedGenerationChangedPredicate[*stnrgwv1.GatewayConfig]{}),
	); err != nil {
		return nil, err
	}
	r.log.Info("Watching GatewayConfig objects")

	// index GatewayConfig objects as per the referenced Secret
	if err := mgr.GetFieldIndexer().IndexField(ctx, &stnrgwv1.GatewayConfig{}, secretGatewayConfigIndex,
		secretGatewayConfigIndexFunc); err != nil {
		return nil, err
	}

	// watch Secret objects referenced by one of our GatewayConfigs
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &corev1.Secret{},
			&handler.TypedEnqueueRequestForObject[*corev1.Secret]{},
			predicate.NewTypedPredicateFuncs[*corev1.Secret](r.validateSecretForReconcile)),
	); err != nil {
		return nil, err
	}
	r.log.Info("Watching Secret objects")

	return r, nil
}

func (r *gatewayConfigReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("resource", req.String())

	if r.terminating {
		r.log.V(2).Info("Controller terminating, suppressing reconciliation")
		return reconcile.Result{}, nil
	}

	log.Info("Reconciling")
	configList := []client.Object{}
	authSecretList := []client.Object{}

	// find all GatewayConfigs
	gcList := &stnrgwv1.GatewayConfigList{}
	if err := r.List(ctx, gcList); err != nil {
		r.log.Info("No GatewayConfigs found")
		return reconcile.Result{}, err
	}

	for _, gc := range gcList.Items {
		gc := gc
		r.log.V(1).Info("Processing GatewayConfig", "name", store.GetObjectKey(&gc))

		configList = append(configList, &gc)

		ref := gc.Spec.AuthRef
		if ref == nil {
			continue
		}

		// obtain ref'd secret
		if (ref.Group != nil && *ref.Group != corev1.GroupName && *ref.Group != "v1") ||
			(ref.Kind != nil && *ref.Kind != "Secret") {
			continue
		}

		namespace := gc.Namespace
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}

		secret := corev1.Secret{}
		secretKey := types.NamespacedName{Namespace: namespace, Name: string(ref.Name)}
		if err := r.Get(ctx, secretKey, &secret); err != nil {
			// not fatal
			if !apierrors.IsNotFound(err) {
				r.log.Error(err, "Error getting Secret", "secret", secretKey)
				continue
			}

			r.log.Info("No Secret found for external auth ref", "GatewayConfig",
				store.GetObjectKey(&gc), "secret", secretKey)

			continue
		}

		r.log.V(1).Info("Found Secret for external auth ref", "GatewayConfig",
			store.GetObjectKey(&gc), "secret", secretKey)

		authSecretList = append(authSecretList, &secret)
	}

	store.GatewayConfigs.Reset(configList)
	r.log.V(2).Info("Reset GatewayConfig store", "configs", store.GatewayConfigs.String())

	store.AuthSecrets.Reset(authSecretList)
	r.log.V(2).Info("Reset AuthSecret store", "secrets", store.AuthSecrets.String())

	if !r.terminating {
		r.eventCh <- event.NewEventReconcile()
	}

	return reconcile.Result{}, nil
}

// validateSecretForReconcile checks whether the Secret belongs to a valid GatewayConfig.
func (r *gatewayConfigReconciler) validateSecretForReconcile(secret *corev1.Secret) bool {
	gcList := &stnrgwv1.GatewayConfigList{}
	secretName := store.GetNamespacedName(secret).String()
	if err := r.List(context.Background(), gcList, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(secretGatewayConfigIndex, secretName),
	}); err != nil {
		r.log.Error(err, "Unable to find associated GatewayConfigs", "secret", secretName)
		return false
	}

	return len(gcList.Items) != 0
}

// secretGatewayConfigIndexFunc indexes GatewayConfigs on the Secret referred via the authRef.
func secretGatewayConfigIndexFunc(o client.Object) []string {
	gatewayConfig := o.(*stnrgwv1.GatewayConfig)
	ret := []string{}

	// authRef not specified
	if gatewayConfig.Spec.AuthRef == nil {
		return ret
	}

	ref := gatewayConfig.Spec.AuthRef

	// - group MUST be set to "" (corev1.GroupName), "v1", or omitted,
	if ref.Group != nil && (string(*ref.Group) != corev1.GroupName && string(*ref.Group) != "v1") {
		return ret
	}

	// - kind MUST be set to "Secret" or omitted,
	if ref.Kind != nil && string(*ref.Kind) != "Secret" {
		return ret
	}

	// - namespace MAY be omitted, in which case it defaults to the namespace of
	//   the GatewayConfig, or it MAY be any valid namespace where the Secret lives.
	namespace := gatewayConfig.Namespace
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	ret = append(ret, types.NamespacedName{
		Namespace: namespace,
		Name:      string(ref.Name),
	}.String())

	return ret
}

func (r *gatewayConfigReconciler) Terminate() {
	r.terminating = true
}
