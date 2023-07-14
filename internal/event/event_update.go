package event

import (
	"fmt"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// render event
type UpdateConf struct {
	GatewayClasses *store.GatewayClassStore
	Gateways       *store.GatewayStore
	UDPRoutes      *store.UDPRouteStore
	Services       *store.ServiceStore
	ConfigMaps     *store.ConfigMapStore
	Deployments    *store.DeploymentStore
}

type EventUpdate struct {
	Type        EventType
	UpsertQueue UpdateConf
	DeleteQueue UpdateConf
	Generation  int
}

// NewEvent returns an empty event
func NewEventUpdate(generation int) *EventUpdate {
	return &EventUpdate{
		Type: EventTypeUpdate,
		UpsertQueue: UpdateConf{
			GatewayClasses: store.NewGatewayClassStore(),
			Gateways:       store.NewGatewayStore(),
			UDPRoutes:      store.NewUDPRouteStore(),
			Services:       store.NewServiceStore(),
			ConfigMaps:     store.NewConfigMapStore(),
			Deployments:    store.NewDeploymentStore(),
		},
		DeleteQueue: UpdateConf{
			GatewayClasses: store.NewGatewayClassStore(),
			Gateways:       store.NewGatewayStore(),
			UDPRoutes:      store.NewUDPRouteStore(),
			Services:       store.NewServiceStore(),
			ConfigMaps:     store.NewConfigMapStore(),
			Deployments:    store.NewDeploymentStore(),
		},
		Generation: generation,
	}
}

func (e *EventUpdate) GetType() EventType {
	return e.Type
}

func (e *EventUpdate) String() string {
	return fmt.Sprintf("%s (gen: %d): upsert-queue: gway-cls: %d, gway: %d, route: %d, svc: %d, confmap: %d, dp: %d / "+
		"delete-queue: gway-cls: %d, gway: %d, route: %d, svc: %d, confmap: %d, dp: %d", e.Type.String(),
		e.Generation, e.UpsertQueue.GatewayClasses.Len(), e.UpsertQueue.Gateways.Len(),
		e.UpsertQueue.UDPRoutes.Len(), e.UpsertQueue.Services.Len(),
		e.UpsertQueue.ConfigMaps.Len(), e.UpsertQueue.Deployments.Len(),
		e.DeleteQueue.GatewayClasses.Len(), e.DeleteQueue.Gateways.Len(),
		e.DeleteQueue.UDPRoutes.Len(), e.DeleteQueue.Services.Len(),
		e.DeleteQueue.ConfigMaps.Len(), e.DeleteQueue.Deployments.Len())
}
