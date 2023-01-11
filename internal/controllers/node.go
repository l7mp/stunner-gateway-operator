package controllers

import (
	"context"
	// "errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// NodeListSize defines how many nodes we visit in one go to find one with a valid external
// address.
const NodeListSize = 10

type nodeReconciler struct {
	client.Client
	eventCh chan event.Event
	log     logr.Logger
}

func RegisterNodeController(mgr manager.Manager, ch chan event.Event, log logr.Logger) error {
	r := &nodeReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("node-controller"),
	}

	c, err := controller.New("node", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	r.log.Info("created node controller")

	if err := c.Watch(
		&source.Kind{Type: &corev1.Node{}},
		&handler.EnqueueRequestForObject{},
		predicate.ResourceVersionChangedPredicate{},
	); err != nil {
		return err
	}
	r.log.Info("watching node objects")

	return nil
}

func (r *nodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("node", req.String())
	log.Info("reconciling")

	var node corev1.Node
	found := true

	// the node being reconciled
	err := r.Get(ctx, req.NamespacedName, &node)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get Node")
			return reconcile.Result{}, err
		}
		found = false
	}

	// the node stored locally
	nodes := store.Nodes.GetAll()
	if len(nodes) > 1 {
		log.Error(err, "internal error: more than one node found in store", "nodes", len(nodes))
		return reconcile.Result{}, err
	}

	var stored *corev1.Node
	if len(nodes) == 1 {
		stored = nodes[0]
	}

	// no action neeeded if a node other than the one stored locally is being deleted, added or
	// modified
	if stored != nil && store.GetNamespacedName(stored) != req.NamespacedName {
		return reconcile.Result{}, nil
	}

	// locally stored node is being deleted
	if !found && stored != nil && store.GetNamespacedName(stored) == req.NamespacedName {
		log.V(1).Info("deleting locally stored node")
		store.Nodes.Remove(req.NamespacedName)
		stored = nil
	}

	// locally stored node is being modified
	if found && stored != nil && store.GetNamespacedName(stored) == req.NamespacedName {
		log.V(1).Info("modifying locally stored node")

		oldAddr := store.GetExternalAddress(stored)
		newAddr := store.GetExternalAddress(&node)

		// address remains the same
		if oldAddr == newAddr {
			return reconcile.Result{}, nil
		}

		if newAddr != "" {
			log.V(2).Info("external node address has changed", "node",
				store.GetObjectKey(&node), "former-address", oldAddr,
				"new-address", newAddr)

			store.Nodes.Upsert(&node)

			r.eventCh <- event.NewEventRender()
			return reconcile.Result{}, nil
		}

		// node address is going away
		store.Nodes.Remove(req.NamespacedName)
	}

	// find a new new node with a workable external address if one the 3 possible cases happens
	// - stored node deleted
	// - stored node modified and the new version has no external address
	// - no node stored
	newNode, addr, err := r.findNodeWithExternalAddress(ctx)
	if err != nil {
		r.log.Error(err, "failed to find node with valid external address")

		r.eventCh <- event.NewEventRender()
		return reconcile.Result{}, nil
	}

	log.V(2).Info("found node with a usable external address", "node",
		store.GetObjectKey(newNode), "address", addr)

	store.Nodes.Upsert(newNode)

	r.eventCh <- event.NewEventRender()
	return reconcile.Result{}, nil
}

func (r *nodeReconciler) findNodeWithExternalAddress(ctx context.Context) (*corev1.Node, string, error) {
	r.log.V(1).Info("trying to find  node with a usable external address")
	count := 0

	// TODO: this will never scale to very large clusters
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		return nil, "", err
	}

	for _, node := range nodes.Items {
		node := node
		r.log.V(2).Info("processing node", "namespace", node.GetNamespace(),
			"name", node.GetName())

		if addr := store.GetExternalAddress(&node); addr != "" {
			return &node, addr, nil
		}

		count = count + 1
	}

	return nil, "", fmt.Errorf("end of node list found after searching through %d nodes", count)
}

// // list only at most NodeListSize number of nodes in one go
// lo := &client.ListOptions{}
// client.Limit(NodeListSize).ApplyToList(lo)

// for true {
// 	nodes := &corev1.NodeList{}
// 	err := r.List(ctx, nodes, lo)
// 	if err != nil {
// 		return nil, "", err
// 	}

// 	if nodes.Size() == 0 {
// 		return nil, "", nil
// 	}

// 	// found new dosage of nodes
// 	for _, node := range nodes.Items {
// 		node := node
// 		r.log.V(2).Info("processing node", "namespace", node.GetNamespace(),
// 			"name", node.GetName())

// 		if addr := store.GetExternalAddress(&node); addr != "" {
// 			return &node, addr, nil
// 		}

// 		count = count + 1
// 	}

// 	// no luck yet: search on
// 	client.Continue(nodes.Continue).ApplyToList(lo)
// }
