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
}

type EventUpdate struct {
	Type        EventType
	UpsertQueue UpdateConf
	DeleteQueue UpdateConf
}

// NewEvent returns an empty event
func NewEventUpdate() *EventUpdate {
	return &EventUpdate{
		Type: EventTypeUpdate,
		UpsertQueue: UpdateConf{
			GatewayClasses: store.NewGatewayClassStore(),
			Gateways:       store.NewGatewayStore(),
			UDPRoutes:      store.NewUDPRouteStore(),
			ConfigMaps:     store.NewConfigMapStore(),
			Services:       store.NewServiceStore(),
		},
		DeleteQueue: UpdateConf{
			GatewayClasses: store.NewGatewayClassStore(),
			Gateways:       store.NewGatewayStore(),
			UDPRoutes:      store.NewUDPRouteStore(),
			Services:       store.NewServiceStore(),
			ConfigMaps:     store.NewConfigMapStore(),
		},
	}
}

func (e *EventUpdate) GetType() EventType {
	return e.Type
}

func (e *EventUpdate) String() string {
	return fmt.Sprintf("%s: upsert-queue: %d gway-clss, %d gway, %d route, %d svcs: %d confmaps / "+
		"delete-queue: %d gway-clss, %d gway, %d route, %d svcs: %d confmaps",
		e.Type.String(), e.UpsertQueue.GatewayClasses.Len(), e.UpsertQueue.Gateways.Len(),
		e.UpsertQueue.UDPRoutes.Len(), e.UpsertQueue.Services.Len(), e.UpsertQueue.ConfigMaps.Len(),
		e.DeleteQueue.GatewayClasses.Len(), e.DeleteQueue.Gateways.Len(),
		e.DeleteQueue.UDPRoutes.Len(), e.DeleteQueue.Services.Len(), e.DeleteQueue.ConfigMaps.Len())
}
