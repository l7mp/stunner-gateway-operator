package renderer

import (
	"github.com/go-logr/logr"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

// RenderContext contains the GatewayClass and the GatewayConfig for the current rendering task,
// plus additional metadata
type RenderContext struct {
	update *event.EventUpdate
	gc     *gwapiv1.GatewayClass
	gwConf *stnrgwv1.GatewayConfig
	dp     *stnrgwv1.Dataplane
	gws    *store.GatewayStore
	log    logr.Logger
}

func NewRenderContext(r *Renderer, gc *gwapiv1.GatewayClass) *RenderContext {
	logger := r.log
	if gc != nil {
		logger = r.log.WithValues("gateway-class", gc.GetName())
	}
	return &RenderContext{
		update: event.NewEventUpdate(r.gen),
		gc:     gc,
		gws:    store.NewGatewayStore(),
		log:    logger,
	}
}

// Merge merges the update queues of two rendering contexts.
func (r *RenderContext) Merge(mergeable *RenderContext) {
	// MUST BE KEPT IN SYNC WITH EventUpdate

	// merge upsert queues
	upsertQueue1 := &r.update.UpsertQueue
	upsertQueue2 := mergeable.update.UpsertQueue
	store.Merge(upsertQueue1.GatewayClasses, upsertQueue2.GatewayClasses)
	store.Merge(upsertQueue1.Gateways, upsertQueue2.Gateways)
	store.Merge(upsertQueue1.UDPRoutes, upsertQueue2.UDPRoutes)
	store.Merge(upsertQueue1.UDPRoutesV1A2, upsertQueue2.UDPRoutesV1A2)
	store.Merge(upsertQueue1.Services, upsertQueue2.Services)
	store.Merge(upsertQueue1.ConfigMaps, upsertQueue2.ConfigMaps)
	store.Merge(upsertQueue1.Deployments, upsertQueue2.Deployments)

	// merge delete queues
	deleteQueue1 := &r.update.DeleteQueue
	deleteQueue2 := mergeable.update.DeleteQueue
	store.Merge(deleteQueue1.GatewayClasses, deleteQueue2.GatewayClasses)
	store.Merge(deleteQueue1.Gateways, deleteQueue2.Gateways)
	store.Merge(deleteQueue1.UDPRoutes, deleteQueue2.UDPRoutes)
	store.Merge(deleteQueue1.UDPRoutesV1A2, deleteQueue2.UDPRoutesV1A2)
	store.Merge(deleteQueue1.Services, deleteQueue2.Services)
	store.Merge(deleteQueue1.ConfigMaps, deleteQueue2.ConfigMaps)
	store.Merge(deleteQueue1.Deployments, deleteQueue2.Deployments)

	// merge the CDS server's config-queue
	r.update.ConfigQueue = append(r.update.ConfigQueue, mergeable.update.ConfigQueue...)
}
