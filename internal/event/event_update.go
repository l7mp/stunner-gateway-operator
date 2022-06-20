package event

import (
	"fmt"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// render event

type EventUpdate struct {
	Type           EventType
	GatewayClasses store.Store
	Gateways       store.Store
	UDPRoutes      store.Store
	ConfigMaps     store.Store
}

// NewEvent returns an empty event
func NewEventUpdate() *EventUpdate {
	return &EventUpdate{
		Type:           EventTypeUpdate,
		GatewayClasses: store.NewStore(),
		Gateways:       store.NewStore(),
		UDPRoutes:      store.NewStore(),
		ConfigMaps:     store.NewStore(),
	}
}

func (e *EventUpdate) GetType() EventType {
	return e.Type
}

func (e *EventUpdate) String() string {
	return fmt.Sprintf("%s: #gway-classes: %d, #gways: %d, #udp-routes: %d, #configmaps: %d",
		e.Type.String(), e.GatewayClasses.Len(), e.Gateways.Len(), e.UDPRoutes.Len(),
		e.ConfigMaps.Len())
}
