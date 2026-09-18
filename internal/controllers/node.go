package controllers

import (
	"context"

	apiequality "k8s.io/apimachinery/pkg/api/equality"

	// "errors"

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
	eventCh chan<- event.Event
	// reported is set once the first reconcile has requested a render, so that the
	// unchanged-address shortcut below never swallows the report the operator's startup gate
	// waits for.
	reported bool
	log      logr.Logger
}

func NewNodeController(mgr manager.Manager, ch chan<- event.Event, log logr.Logger) (Controller, error) {
	r := &nodeReconciler{
		Client:  mgr.GetClient(),
		eventCh: ch,
		log:     log.WithName("node-controller"),
	}

	c, err := controller.New("node", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}

	r.log.Info("created node controller")

	if err := c.Watch(
		source.Kind(mgr.GetCache(), &corev1.Node{},
			&handler.TypedEnqueueRequestForObject[*corev1.Node]{},
			predicate.TypedResourceVersionChangedPredicate[*corev1.Node]{}),
	); err != nil {
		return nil, err
	}
	r.log.Info("watching node objects")

	return r, nil
}

func (r *nodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("node", req.String())

	log.Info("Reconciling")

	// the node being reconciled
	node := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get Node")
			return reconcile.Result{}, err
		}
		log.Info("node removed: triggering reconcile")
		store.Nodes.Remove(req.NamespacedName)

		r.reported = true
		event.Send(ctx, r.eventCh, event.NewEventReconcile(string(r.Name())))
		return reconcile.Result{}, nil
	}

	storedNode := store.Nodes.GetObject(req.NamespacedName)
	if storedNode == nil {
		log.Info("node added: triggering reconcile")
		store.Nodes.Upsert(node)

		r.reported = true
		event.Send(ctx, r.eventCh, event.NewEventReconcile(string(r.Name())))
		return reconcile.Result{}, nil

	}

	// only reconcile if addresses have changed, except for the first report after start
	if apiequality.Semantic.DeepEqual(storedNode.Status.Addresses, node.Status.Addresses) && r.reported {
		// ignore event
		return reconcile.Result{}, nil
	}

	log.Info("node addresses changed or first report: triggering reconcile")
	store.Nodes.Upsert(node)

	r.reported = true
	event.Send(ctx, r.eventCh, event.NewEventReconcile(string(r.Name())))
	return reconcile.Result{}, nil
}

func (r *nodeReconciler) Name() ControllerName { return NodeControllerName }
