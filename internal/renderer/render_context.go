package renderer

import (
	"github.com/go-logr/logr"

	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// RenderContext contains the GatewayClass and the GatewayConfig for the current rendering task,
// plus additional metadata
type RenderContext struct {
	origin event.Event
	update *event.EventUpdate
	gc     *gwapiv1b1.GatewayClass
	gwConf *stnrv1a1.GatewayConfig
	gws    *store.GatewayStore
	log    logr.Logger
}

func NewRenderContext(e *event.EventRender, r *Renderer, gc *gwapiv1b1.GatewayClass) *RenderContext {
	return &RenderContext{
		origin: e,
		update: event.NewEventUpdate(r.gen),
		gc:     gc,
		gws:    store.NewGatewayStore(),
		log:    r.log.WithValues("gateway-class", gc.GetName()),
	}
}

// Merge merges the update queues of two rendering contexts.
func (r *RenderContext) Merge(mergeable *RenderContext) {
	if store.GetObjectKey(r.gc) != store.GetObjectKey(mergeable.gc) ||
		store.GetObjectKey(r.gwConf) != store.GetObjectKey(mergeable.gwConf) {
		panic("MergeUpdateQueue: trying to merge incompatible render contexts")
	}

	// MUST BE KEPT IN SYNC WITH EventUpdate

	// merge upsert queues
	upsertQueue1 := &r.update.UpsertQueue
	upsertQueue2 := mergeable.update.UpsertQueue
	store.Merge(upsertQueue1.GatewayClasses, upsertQueue2.GatewayClasses)
	store.Merge(upsertQueue1.Gateways, upsertQueue2.Gateways)
	store.Merge(upsertQueue1.UDPRoutes, upsertQueue2.UDPRoutes)
	store.Merge(upsertQueue1.Services, upsertQueue2.Services)
	store.Merge(upsertQueue1.ConfigMaps, upsertQueue2.ConfigMaps)
	store.Merge(upsertQueue1.Deployments, upsertQueue2.Deployments)

	// merge delete queues
	deleteQueue1 := &r.update.DeleteQueue
	deleteQueue2 := mergeable.update.DeleteQueue
	store.Merge(deleteQueue1.GatewayClasses, deleteQueue2.GatewayClasses)
	store.Merge(deleteQueue1.Gateways, deleteQueue2.Gateways)
	store.Merge(deleteQueue1.UDPRoutes, deleteQueue2.UDPRoutes)
	store.Merge(deleteQueue1.Services, deleteQueue2.Services)
	store.Merge(deleteQueue1.ConfigMaps, deleteQueue2.ConfigMaps)
	store.Merge(deleteQueue1.Deployments, deleteQueue2.Deployments)
}
