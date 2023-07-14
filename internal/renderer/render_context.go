package renderer

import (
	"github.com/go-logr/logr"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

// RenderContext contains the GatewayClass and the GatewayConfig for the current rendering task,
// plus additional metadata
type RenderContext struct {
	origin event.Event
	update *event.EventUpdate
	gc     *gwapiv1a2.GatewayClass
	gwConf *stnrv1a1.GatewayConfig
	gws    *store.GatewayStore
	log    logr.Logger
}

func NewRenderContext(e *event.EventRender, r *Renderer, gc *gwapiv1a2.GatewayClass) *RenderContext {
	return &RenderContext{
		origin: e,
		update: event.NewEventUpdate(r.gen),
		gc:     gc,
		gws:    store.NewGatewayStore(),
		log:    r.log.WithValues("gateway-class", gc.GetName()),
	}

}
