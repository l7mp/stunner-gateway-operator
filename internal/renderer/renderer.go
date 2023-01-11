package renderer

import (
	"context"
	// "fmt"
	// "reflect"

	"github.com/go-logr/logr"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"k8s.io/apimachinery/pkg/runtime"

	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// corev1 "k8s.io/api/core/v1"

	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/updater"
	// stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

type RendererConfig struct {
	Scheme *runtime.Scheme
	Logger logr.Logger
}

type Renderer struct {
	ctx                  context.Context
	scheme               *runtime.Scheme
	gen                  int
	renderCh, operatorCh chan event.Event
	log                  logr.Logger
}

// NewRenderer creates a new Renderer
func NewRenderer(cfg RendererConfig) *Renderer {
	return &Renderer{
		scheme:   cfg.Scheme,
		renderCh: make(chan event.Event, 10),
		gen:      0,
		log:      cfg.Logger.WithName("renderer"),
	}
}

func (r *Renderer) Start(ctx context.Context) error {
	r.ctx = ctx

	// starting the renderer thread
	go func() {
		defer close(r.renderCh)

		for {
			select {
			case e := <-r.renderCh:
				if e.GetType() != event.EventTypeRender {
					r.log.Info("renderer thread received unknown event",
						"event", e.String())
					continue
				}

				// prepare a new update event Render will populate
				// config is returned in the update event ConfigMap store
				ev := e.(*event.EventRender)
				r.Render(ev)

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// GetRenderChannel returns the channel onn which the renderer listenens to rendering requests
func (r *Renderer) GetRenderChannel() chan event.Event {
	return r.renderCh
}

// SetOperatorChannel sets the channel on which the operator event dispatcher listens
func (r *Renderer) SetOperatorChannel(ch chan event.Event) {
	r.operatorCh = ch
}
