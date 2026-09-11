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
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Controller interface {
	Reconcile(context.Context, reconcile.Request) (reconcile.Result, error)
	Terminate()
}

// serializedController also serializes the initial HA snapshot with controller-runtime
// reconciliation. A slower initial List must not overwrite a newer reconciliation.
type serializedController struct {
	Controller
	mu sync.Mutex
}

func serialize(c Controller) Controller { return &serializedController{Controller: c} }

func (c *serializedController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Controller.Reconcile(ctx, req)
}

func (c *serializedController) Terminate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Controller.Terminate()
}
