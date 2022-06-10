package implementation

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-kubernetes-gateway/internal/config"
	"github.com/l7mp/stunner-kubernetes-gateway/internal/events"
	"github.com/l7mp/stunner-kubernetes-gateway/pkg/sdk/v1alpha2"
)

type udpRouteImplementation struct {
	conf    config.Config
	eventCh chan<- interface{}
}

// NewUDPRouteImplementation creates a new UDPRouteImplementation.
func NewUDPRouteImplementation(cfg config.Config, eventCh chan<- interface{}) sdk.UDPRouteImpl {
	return &udpRouteImplementation{
		conf:    cfg,
		eventCh: eventCh,
	}
}

func (impl *udpRouteImplementation) Logger() logr.Logger {
	return impl.conf.Logger
}

func (impl *udpRouteImplementation) ControllerName() string {
	return impl.conf.GatewayCtlrName
}

func (impl *udpRouteImplementation) Upsert(hr *v1alpha2.UDPRoute) {
	impl.Logger().Info("UDPRoute was upserted",
		"namespace", hr.Namespace, "name", hr.Name,
	)

	impl.eventCh <- &events.UpsertEvent{
		Resource: hr,
	}
}

func (impl *udpRouteImplementation) Remove(nsname types.NamespacedName) {
	impl.Logger().Info("UDPRoute resource was removed",
		"namespace", nsname.Namespace, "name", nsname.Name,
	)

	impl.eventCh <- &events.DeleteEvent{
		NamespacedName: nsname,
		Type:           &v1alpha2.UDPRoute{},
	}
}
