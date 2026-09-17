package event

import (
	"fmt"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// render event
type ConfigConf = []*stnrapiv1.StunnerConfig
type UpdateConf struct {
	GatewayClasses store.Store
	Gateways       store.Store
	UDPRoutes      store.Store
	UDPRoutesGwAPI store.Store
	TCPRoutes      store.Store
	TCPRoutesGwAPI store.Store
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
	LicenseStatus stnrapiv1.LicenseStatus
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
			UDPRoutesGwAPI: store.NewStore(),
			TCPRoutes:      store.NewStore(),
			TCPRoutesGwAPI: store.NewStore(),
			Services:       store.NewStore(),
			ConfigMaps:     store.NewStore(),
			Deployments:    store.NewStore(),
			DaemonSets:     store.NewStore(),
		},
		DeleteQueue: UpdateConf{
			GatewayClasses: store.NewStore(),
			Gateways:       store.NewStore(),
			UDPRoutes:      store.NewStore(),
			UDPRoutesGwAPI: store.NewStore(),
			TCPRoutes:      store.NewStore(),
			TCPRoutesGwAPI: store.NewStore(),
			Services:       store.NewStore(),
			ConfigMaps:     store.NewStore(),
			Deployments:    store.NewStore(),
			DaemonSets:     store.NewStore(),
		},
		ConfigQueue:   []*stnrapiv1.StunnerConfig{},
		LicenseStatus: stnrapiv1.NewEmptyLicenseStatus(),
		Generation:    generation,
		RequestAck:    false,
	}
}

func (e *EventUpdate) GetType() EventType {
	return e.Type
}

func (e *EventUpdate) String() string {
	return fmt.Sprintf("%s (gen: %d, ack: %t, license: %s): upsert-queue: gway-cls: %d, gway: %d, "+
		"udp-route: %d, udp-routeV1A2: %d, tcp-route: %d, tcp-routeV1: %d, "+
		"svc: %d, confmap: %d, dp: %d, ds: %d / "+
		"delete-queue: gway-cls: %d, gway: %d, udp-route: %d, udp-routeV1A2: %d, "+
		"tcp-route: %d, tcp-routeV1: %d, "+
		"svc: %d, confmap: %d, dp: %d, ds: %d / config-queue: %d",
		e.Type.String(), e.Generation, e.RequestAck, e.LicenseStatus.String(),
		e.UpsertQueue.GatewayClasses.Len(), e.UpsertQueue.Gateways.Len(),
		e.UpsertQueue.UDPRoutes.Len(), e.UpsertQueue.UDPRoutesGwAPI.Len(),
		e.UpsertQueue.TCPRoutes.Len(), e.UpsertQueue.TCPRoutesGwAPI.Len(),
		e.UpsertQueue.Services.Len(), e.UpsertQueue.ConfigMaps.Len(),
		e.UpsertQueue.Deployments.Len(), e.UpsertQueue.DaemonSets.Len(),
		e.DeleteQueue.GatewayClasses.Len(), e.DeleteQueue.Gateways.Len(),
		e.DeleteQueue.UDPRoutes.Len(), e.DeleteQueue.UDPRoutesGwAPI.Len(),
		e.DeleteQueue.TCPRoutes.Len(), e.DeleteQueue.TCPRoutesGwAPI.Len(),
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
	u.UpsertQueue.UDPRoutesGwAPI = deepCopyStore(q.UDPRoutesGwAPI)
	u.UpsertQueue.TCPRoutes = deepCopyStore(q.TCPRoutes)
	u.UpsertQueue.TCPRoutesGwAPI = deepCopyStore(q.TCPRoutesGwAPI)
	u.UpsertQueue.Services = deepCopyStore(q.Services)
	u.UpsertQueue.ConfigMaps = deepCopyStore(q.ConfigMaps)
	u.UpsertQueue.Deployments = deepCopyStore(q.Deployments)
	u.UpsertQueue.DaemonSets = deepCopyStore(q.DaemonSets)

	q = e.DeleteQueue
	u.DeleteQueue.GatewayClasses = deepCopyStore(q.GatewayClasses)
	u.DeleteQueue.Gateways = deepCopyStore(q.Gateways)
	u.DeleteQueue.UDPRoutes = deepCopyStore(q.UDPRoutes)
	u.DeleteQueue.UDPRoutesGwAPI = deepCopyStore(q.UDPRoutesGwAPI)
	u.DeleteQueue.TCPRoutes = deepCopyStore(q.TCPRoutes)
	u.DeleteQueue.TCPRoutesGwAPI = deepCopyStore(q.TCPRoutesGwAPI)
	u.DeleteQueue.Services = deepCopyStore(q.Services)
	u.DeleteQueue.ConfigMaps = deepCopyStore(q.ConfigMaps)
	u.DeleteQueue.Deployments = deepCopyStore(q.Deployments)
	u.DeleteQueue.DaemonSets = deepCopyStore(q.DaemonSets)

	u.LicenseStatus = e.LicenseStatus

	u.ConfigQueue = make([]*stnrapiv1.StunnerConfig, len(e.ConfigQueue))
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
