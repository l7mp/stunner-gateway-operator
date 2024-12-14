package renderer

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	licensemgr "github.com/l7mp/stunner-gateway-operator/internal/licensemanager"
)

type RendererConfig struct {
	Scheme         *runtime.Scheme
	LicenseManager licensemgr.Manager
	Logger         logr.Logger
}

type Renderer struct {
	ctx                  context.Context
	scheme               *runtime.Scheme
	licmgr               licensemgr.Manager
	gen                  int
	renderCh, operatorCh chan event.Event
	*config.ProgressTracker
	log logr.Logger
}

// NewRenderer creates a new Renderer.
func NewRenderer(cfg RendererConfig) *Renderer {
	return &Renderer{
		scheme:          cfg.Scheme,
		licmgr:          cfg.LicenseManager,
		renderCh:        make(chan event.Event, 10),
		gen:             0,
		ProgressTracker: config.NewProgressTracker(),
		log:             cfg.Logger.WithName("renderer"),
	}
}

func (r *Renderer) Start(ctx context.Context) error {
	r.ctx = ctx

	go func() {
		defer close(r.renderCh)

		for {
			select {
			case e := <-r.renderCh:
				switch e.GetType() {
				case event.EventTypeRender:
					// prepare a new update event Render will populate config
					// is returned in the update event ConfigMap store
					ev := e.(*event.EventRender)

					r.ProgressUpdate(1)
					r.Render(ev)
					r.ProgressUpdate(-1)
				case event.EventTypeFinalize:
					// invaliditate all statuses and configs
					ev := e.(*event.EventFinalize)

					r.ProgressUpdate(1)
					r.Finalize(ev)
					r.ProgressUpdate(-1)
				default:
					r.log.Info("Renderer thread received unknown event", "event", e.String())
				}
				continue

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// GetRenderChannel returns the channel onn which the renderer listenens to rendering requests.
func (r *Renderer) GetRenderChannel() chan event.Event {
	return r.renderCh
}

// SetOperatorChannel sets the channel on which the operator event dispatcher listens.
func (r *Renderer) SetOperatorChannel(ch chan event.Event) {
	r.operatorCh = ch
}
