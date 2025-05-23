package renderer

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	licensemgr "github.com/l7mp/stunner-gateway-operator/internal/licensemanager"
)

var NewRenderer = NewDefaultRenderer

type Renderer interface {
	config.ProgressReporter
	Start(ctx context.Context) error
	GetRenderChannel() chan event.Event
	SetOperatorChannel(ch event.EventChannel)
}

// configRenderer is a generic interface for the rendering components that can generate components
// of the dataplane config.
type configRenderer interface {
	render(c *RenderContext, args ...any) (stnrconfv1.Config, error)
}

// resourceGenerator is a generic interface for the generator components that can create K8s
// resources.
type resourceGenerator interface {
	generate(c *RenderContext) (client.Object, error)
}

type RendererConfig struct {
	Scheme         *runtime.Scheme
	LicenseManager licensemgr.Manager
	Logger         logr.Logger
}

type renderer struct {
	ctx                                           context.Context
	scheme                                        *runtime.Scheme
	licmgr                                        licensemgr.Manager
	adminRenderer, authRenderer, listenerRenderer configRenderer
	dataplaneGenerator                            resourceGenerator
	gen                                           int
	renderCh                                      chan event.Event
	operatorCh                                    event.EventChannel
	*config.ProgressTracker
	log logr.Logger
}

// NewDefaultRenderer creates a new default Renderer.
func NewDefaultRenderer(cfg RendererConfig) Renderer {
	r := &renderer{
		scheme:             cfg.Scheme,
		licmgr:             cfg.LicenseManager,
		adminRenderer:      newAdminRenderer(),
		authRenderer:       newAuthRenderer(),
		listenerRenderer:   newListenerRenderer(cfg.Logger.WithName("listener-renderer")),
		dataplaneGenerator: newDataplaneGenerator(cfg.Scheme),
		renderCh:           make(chan event.Event, 10),
		gen:                0,
		ProgressTracker:    config.NewProgressTracker(),
		log:                cfg.Logger.WithName("renderer"),
	}
	r.log.V(4).Info("Renderer thread created (**default** renderer)")
	return r
}

func (r *renderer) Start(ctx context.Context) error {
	r.ctx = ctx

	go func() {
		defer func() {
			close(r.renderCh)
			if r.operatorCh != nil {
				r.operatorCh.Put()
			}
		}()

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
func (r *renderer) GetRenderChannel() chan event.Event {
	return r.renderCh
}

// SetOperatorChannel sets the channel on which the operator event dispatcher listens.
func (r *renderer) SetOperatorChannel(ch event.EventChannel) {
	r.operatorCh = ch
	ch.Get()
}

// renderAdmin is a wrapper for adminRenderer.render()
func (r *renderer) renderAdmin(c *RenderContext) (*stnrconfv1.AdminConfig, error) {
	conf, err := r.adminRenderer.render(c)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrconfv1.AdminConfig), nil
}

// renderAuth is a wrapper for authRenderer.render()
func (r *renderer) renderAuth(c *RenderContext) (*stnrconfv1.AuthConfig, error) {
	conf, err := r.authRenderer.render(c)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrconfv1.AuthConfig), nil
}

// renderListener is a wrapper for listenerRenderer.render()
func (r *renderer) renderListener(c *RenderContext, l *gwapiv1.Listener, rs []*stnrgwv1.UDPRoute, ap gwAddrPort, targetPorts map[string]int) (*stnrconfv1.ListenerConfig, error) {
	conf, err := r.listenerRenderer.render(c, l, rs, ap, targetPorts)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrconfv1.ListenerConfig), nil
}

// generateDataplane is a wrapper for dataplaneGenerator.generate()
func (r *renderer) generateDataplane(c *RenderContext) (client.Object, error) {
	obj, err := r.dataplaneGenerator.generate(c)
	if err != nil {
		return nil, err
	}
	return obj, nil
}
