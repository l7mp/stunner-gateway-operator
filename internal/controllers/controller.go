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

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
