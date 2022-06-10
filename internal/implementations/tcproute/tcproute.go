package implementation

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-kubernetes-gateway/internal/config"
	"github.com/l7mp/stunner-kubernetes-gateway/internal/events"
	"github.com/l7mp/stunner-kubernetes-gateway/pkg/sdk/v1alpha2"
)

type tcpRouteImplementation struct {
	conf    config.Config
	eventCh chan<- interface{}
}

// NewTCPRouteImplementation creates a new TCPRouteImplementation.
func NewTCPRouteImplementation(cfg config.Config, eventCh chan<- interface{}) sdk.TCPRouteImpl {
	return &tcpRouteImplementation{
		conf:    cfg,
		eventCh: eventCh,
	}
}

func (impl *tcpRouteImplementation) Logger() logr.Logger {
	return impl.conf.Logger
}

func (impl *tcpRouteImplementation) ControllerName() string {
	return impl.conf.GatewayCtlrName
}

func (impl *tcpRouteImplementation) Upsert(hr *v1alpha2.TCPRoute) {
	impl.Logger().Info("TCPRoute was upserted",
		"namespace", hr.Namespace, "name", hr.Name,
	)

	impl.eventCh <- &events.UpsertEvent{
		Resource: hr,
	}
}

func (impl *tcpRouteImplementation) Remove(nsname types.NamespacedName) {
	impl.Logger().Info("TCPRoute resource was removed",
		"namespace", nsname.Namespace, "name", nsname.Name,
	)

	impl.eventCh <- &events.DeleteEvent{
		NamespacedName: nsname,
		Type:           &v1alpha2.TCPRoute{},
	}
}
