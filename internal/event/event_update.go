package event

import (
	"fmt"

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// render event
type ConfigConf = []*stnrv1.StunnerConfig
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
	ConfigQueue ConfigConf
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
		ConfigQueue: []*stnrv1.StunnerConfig{},
		Generation:  generation,
	}
}

func (e *EventUpdate) GetType() EventType {
	return e.Type
}

func (e *EventUpdate) String() string {
	return fmt.Sprintf("%s (gen: %d): upsert-queue: gway-cls: %d, gway: %d, route: %d, svc: %d, confmap: %d, dp: %d / "+
		"delete-queue: gway-cls: %d, gway: %d, route: %d, svc: %d, confmap: %d, dp: %d / config-queue: %d",
		e.Type.String(),
		e.Generation, e.UpsertQueue.GatewayClasses.Len(), e.UpsertQueue.Gateways.Len(),
		e.UpsertQueue.UDPRoutes.Len(), e.UpsertQueue.Services.Len(),
		e.UpsertQueue.ConfigMaps.Len(), e.UpsertQueue.Deployments.Len(),
		e.DeleteQueue.GatewayClasses.Len(), e.DeleteQueue.Gateways.Len(),
		e.DeleteQueue.UDPRoutes.Len(), e.DeleteQueue.Services.Len(),
		e.DeleteQueue.ConfigMaps.Len(), e.DeleteQueue.Deployments.Len(),
		len(e.ConfigQueue))
}
