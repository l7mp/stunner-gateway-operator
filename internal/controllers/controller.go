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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// secretMetadata is the object the Secret watches take: the operator watches Secrets by their
// metadata only and reads the ones it references from the API server.
func secretMetadata() *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}}
}

// ControllerName identifies a controller in the reconcile events it sends to the operator.
type ControllerName string

const (
	GatewayConfigControllerName ControllerName = "gatewayconfig"
	DataplaneControllerName     ControllerName = "dataplane"
	GatewayControllerName       ControllerName = "gateway"
	RouteControllerName         ControllerName = "route"
	NodeControllerName          ControllerName = "node"
)

// Controller is a reconciler that refreshes the stores of its kinds from the cache on every
// reconcile and then asks the operator for a rendering round, signed with its name.
type Controller interface {
	Name() ControllerName
	Reconcile(context.Context, reconcile.Request) (reconcile.Result, error)
}

// isServed tells whether the API server serves the kind of obj, for the controllers whose CRDs
// are optional.
func isServed(mgr manager.Manager, obj client.Object) (bool, error) {
	gvk, err := apiutil.GVKForObject(obj, mgr.GetScheme())
	if err != nil {
		return false, fmt.Errorf("failed to get GVK for %T: %w", obj, err)
	}
	if _, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version); err != nil {
		if meta.IsNoMatchError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to map %s: %w", gvk, err)
	}
	return true, nil
}
