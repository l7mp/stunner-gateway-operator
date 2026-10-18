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

func NewRenderContext(r *renderer, gc *gwapiv1.GatewayClass) *RenderContext {
	logger := r.log
	if gc != nil {
		logger = r.log.WithValues("gateway-class", gc.GetName())
	}
	update := event.NewEventUpdate(r.gen)
	update.LicenseStatus = r.licmgr.Status()

	return &RenderContext{
		update: update,
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
	store.Merge(upsertQueue1.UDPRoutesGwAPI, upsertQueue2.UDPRoutesGwAPI)
	store.Merge(upsertQueue1.TCPRoutes, upsertQueue2.TCPRoutes)
	store.Merge(upsertQueue1.TCPRoutesGwAPI, upsertQueue2.TCPRoutesGwAPI)
	store.Merge(upsertQueue1.TURNRoutes, upsertQueue2.TURNRoutes)
	store.Merge(upsertQueue1.Services, upsertQueue2.Services)
	store.Merge(upsertQueue1.Deployments, upsertQueue2.Deployments)
	store.Merge(upsertQueue1.DaemonSets, upsertQueue2.DaemonSets)

	// merge delete queues
	deleteQueue1 := &r.update.DeleteQueue
	deleteQueue2 := mergeable.update.DeleteQueue
	store.Merge(deleteQueue1.GatewayClasses, deleteQueue2.GatewayClasses)
	store.Merge(deleteQueue1.Gateways, deleteQueue2.Gateways)
	store.Merge(deleteQueue1.UDPRoutes, deleteQueue2.UDPRoutes)
	store.Merge(deleteQueue1.UDPRoutesGwAPI, deleteQueue2.UDPRoutesGwAPI)
	store.Merge(deleteQueue1.TCPRoutes, deleteQueue2.TCPRoutes)
	store.Merge(deleteQueue1.TCPRoutesGwAPI, deleteQueue2.TCPRoutesGwAPI)
	store.Merge(deleteQueue1.TURNRoutes, deleteQueue2.TURNRoutes)
	store.Merge(deleteQueue1.Services, deleteQueue2.Services)
	store.Merge(deleteQueue1.Deployments, deleteQueue2.Deployments)
	store.Merge(deleteQueue1.DaemonSets, deleteQueue2.DaemonSets)

	// merge the CDS server's config-queue
	r.update.ConfigQueue = append(r.update.ConfigQueue, mergeable.update.ConfigQueue...)
}
