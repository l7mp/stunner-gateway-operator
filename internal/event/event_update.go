package event

import (
	"fmt"

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// render event
type ConfigConf = []*stnrv1.StunnerConfig
type UpdateConf struct {
	GatewayClasses store.Store
	Gateways       store.Store
	UDPRoutes      store.Store
	UDPRoutesV1A2  store.Store
	Services       store.Store
	ConfigMaps     store.Store
	Deployments    store.Store
	DaemonSets     store.Store
}

type EventUpdate struct {
	Type          EventType
	UpsertQueue   UpdateConf
	DeleteQueue   UpdateConf
	ConfigQueue   ConfigConf
	LicenseStatus stnrv1.LicenseStatus
	Generation    int
	RequestAck    bool
}

// NewEvent returns an empty event
func NewEventUpdate(generation int) *EventUpdate {
	return &EventUpdate{
		Type: EventTypeUpdate,
		UpsertQueue: UpdateConf{
			GatewayClasses: store.NewStore(),
			Gateways:       store.NewStore(),
			UDPRoutes:      store.NewStore(),
			UDPRoutesV1A2:  store.NewStore(),
			Services:       store.NewStore(),
			ConfigMaps:     store.NewStore(),
			Deployments:    store.NewStore(),
			DaemonSets:     store.NewStore(),
		},
		DeleteQueue: UpdateConf{
			GatewayClasses: store.NewStore(),
			Gateways:       store.NewStore(),
			UDPRoutes:      store.NewStore(),
			UDPRoutesV1A2:  store.NewStore(),
			Services:       store.NewStore(),
			ConfigMaps:     store.NewStore(),
			Deployments:    store.NewStore(),
			DaemonSets:     store.NewStore(),
		},
		ConfigQueue:   []*stnrv1.StunnerConfig{},
		LicenseStatus: stnrv1.NewEmptyLicenseStatus(),
		Generation:    generation,
		RequestAck:    false,
	}
}

func (e *EventUpdate) GetType() EventType {
	return e.Type
}

func (e *EventUpdate) String() string {
	return fmt.Sprintf("%s (gen: %d, ack: %t, license: %s): upsert-queue: gway-cls: %d, gway: %d, "+
		"route: %d, routeV1A2: %d, svc: %d, confmap: %d, dp: %d, ds: %d / "+
		"delete-queue: gway-cls: %d, gway: %d, route: %d, routeV1A2: %d, "+
		"svc: %d, confmap: %d, dp: %d, ds: %d / config-queue: %d",
		e.Type.String(), e.Generation, e.RequestAck, e.LicenseStatus.String(),
		e.UpsertQueue.GatewayClasses.Len(), e.UpsertQueue.Gateways.Len(),
		e.UpsertQueue.UDPRoutes.Len(), e.UpsertQueue.UDPRoutesV1A2.Len(),
		e.UpsertQueue.Services.Len(), e.UpsertQueue.ConfigMaps.Len(),
		e.UpsertQueue.Deployments.Len(), e.UpsertQueue.DaemonSets.Len(),
		e.DeleteQueue.GatewayClasses.Len(), e.DeleteQueue.Gateways.Len(),
		e.DeleteQueue.UDPRoutes.Len(), e.DeleteQueue.UDPRoutesV1A2.Len(),
		e.DeleteQueue.Services.Len(), e.DeleteQueue.ConfigMaps.Len(),
		e.DeleteQueue.Deployments.Len(), e.DeleteQueue.DaemonSets.Len(),
		len(e.ConfigQueue))
}

// DeepCopy copies all updated resources into a new update event. This is required to elide locking
// across the renderer thread and the updater thread.
func (e *EventUpdate) DeepCopy() *EventUpdate {
	u := NewEventUpdate(e.Generation)

	q := e.UpsertQueue
	u.UpsertQueue.GatewayClasses = deepCopyStore(q.GatewayClasses)
	u.UpsertQueue.Gateways = deepCopyStore(q.Gateways)
	u.UpsertQueue.UDPRoutes = deepCopyStore(q.UDPRoutes)
	u.UpsertQueue.UDPRoutesV1A2 = deepCopyStore(q.UDPRoutesV1A2)
	u.UpsertQueue.Services = deepCopyStore(q.Services)
	u.UpsertQueue.ConfigMaps = deepCopyStore(q.ConfigMaps)
	u.UpsertQueue.Deployments = deepCopyStore(q.Deployments)
	u.UpsertQueue.DaemonSets = deepCopyStore(q.DaemonSets)

	q = e.DeleteQueue
	u.DeleteQueue.GatewayClasses = deepCopyStore(q.GatewayClasses)
	u.DeleteQueue.Gateways = deepCopyStore(q.Gateways)
	u.DeleteQueue.UDPRoutes = deepCopyStore(q.UDPRoutes)
	u.DeleteQueue.UDPRoutesV1A2 = deepCopyStore(q.UDPRoutesV1A2)
	u.DeleteQueue.Services = deepCopyStore(q.Services)
	u.DeleteQueue.ConfigMaps = deepCopyStore(q.ConfigMaps)
	u.DeleteQueue.Deployments = deepCopyStore(q.Deployments)
	u.DeleteQueue.DaemonSets = deepCopyStore(q.DaemonSets)

	u.LicenseStatus = e.LicenseStatus

	u.ConfigQueue = make([]*stnrv1.StunnerConfig, len(e.ConfigQueue))
	copy(u.ConfigQueue, e.ConfigQueue)

	return u
}

// GetRequestAck returns true of the event contains an acknowledgement request.
func (e *EventUpdate) GetRequestAck() bool {
	return e.RequestAck
}

// RequestAck asks the updater to send an acknowledgement.
func (e *EventUpdate) SetRequestAck(b bool) {
	e.RequestAck = b
}

func deepCopyStore(s store.Store) store.Store {
	ret := store.NewStore()
	for _, o := range s.Objects() {
		copy, ok := o.DeepCopyObject().(client.Object)
		if !ok {
			panic(fmt.Sprintf("cannot deepcopy object %T", o))
		}
		ret.Upsert(copy)
	}

	return ret
}
