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
	UDPRoutesV1A2  *store.UDPRouteStore
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
			UDPRoutesV1A2:  store.NewUDPRouteStore(),
			Services:       store.NewServiceStore(),
			ConfigMaps:     store.NewConfigMapStore(),
			Deployments:    store.NewDeploymentStore(),
		},
		DeleteQueue: UpdateConf{
			GatewayClasses: store.NewGatewayClassStore(),
			Gateways:       store.NewGatewayStore(),
			UDPRoutes:      store.NewUDPRouteStore(),
			UDPRoutesV1A2:  store.NewUDPRouteStore(),
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
	return fmt.Sprintf("%s (gen: %d): upsert-queue: gway-cls: %d, gway: %d, route: %d, routeV1A2: %d, svc: %d, confmap: %d, dp: %d / "+
		"delete-queue: gway-cls: %d, gway: %d, route: %d, routeV1A2: %d, svc: %d, confmap: %d, dp: %d / config-queue: %d",
		e.Type.String(), e.Generation,
		e.UpsertQueue.GatewayClasses.Len(), e.UpsertQueue.Gateways.Len(),
		e.UpsertQueue.UDPRoutes.Len(), e.UpsertQueue.UDPRoutesV1A2.Len(),
		e.UpsertQueue.Services.Len(), e.UpsertQueue.ConfigMaps.Len(),
		e.UpsertQueue.Deployments.Len(),
		e.DeleteQueue.GatewayClasses.Len(), e.DeleteQueue.Gateways.Len(),
		e.DeleteQueue.UDPRoutes.Len(), e.DeleteQueue.UDPRoutesV1A2.Len(),
		e.DeleteQueue.Services.Len(), e.DeleteQueue.ConfigMaps.Len(),
		e.DeleteQueue.Deployments.Len(),
		len(e.ConfigQueue))
}

// DeepCopy copies all updated resources into a new update event. This is required to elide locking
// across the renderer thread and the updater thread.
func (e *EventUpdate) DeepCopy() *EventUpdate {
	u := NewEventUpdate(e.Generation)

	q := e.UpsertQueue
	u.UpsertQueue.GatewayClasses = q.GatewayClasses.DeepCopy()
	u.UpsertQueue.Gateways = q.Gateways.DeepCopy()
	u.UpsertQueue.UDPRoutes = q.UDPRoutes.DeepCopy()
	u.UpsertQueue.UDPRoutesV1A2 = q.UDPRoutesV1A2.DeepCopy()
	u.UpsertQueue.Services = q.Services.DeepCopy()
	u.UpsertQueue.ConfigMaps = q.ConfigMaps.DeepCopy()
	u.UpsertQueue.Deployments = q.Deployments.DeepCopy()

	q = e.DeleteQueue
	u.DeleteQueue.GatewayClasses = q.GatewayClasses.DeepCopy()
	u.DeleteQueue.Gateways = q.Gateways.DeepCopy()
	u.DeleteQueue.UDPRoutes = q.UDPRoutes.DeepCopy()
	u.DeleteQueue.UDPRoutesV1A2 = q.UDPRoutesV1A2.DeepCopy()
	u.DeleteQueue.Services = q.Services.DeepCopy()
	u.DeleteQueue.ConfigMaps = q.ConfigMaps.DeepCopy()
	u.DeleteQueue.Deployments = q.Deployments.DeepCopy()

	u.ConfigQueue = make([]*stnrv1.StunnerConfig, len(e.ConfigQueue))
	copy(u.ConfigQueue, e.ConfigQueue)

	return u
}
