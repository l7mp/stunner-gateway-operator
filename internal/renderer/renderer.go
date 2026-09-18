package renderer

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	licensemgr "github.com/l7mp/stunner-gateway-operator/internal/licensemanager"
	"github.com/l7mp/stunner-gateway-operator/internal/metrics"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

var NewRenderer = NewDefaultRenderer

// Renderer turns the Gateway API state in the stores into dataplane configs and Kubernetes
// resources. It runs as a leader-only manager runnable: Start blocks until the context ends.
type Renderer interface {
	config.ProgressReporter
	Start(ctx context.Context) error
	GetRenderChannel() chan event.Event
	SetOperatorChannel(ch chan<- event.Event)
	// Finalize invalidates every managed resource for the given generation and returns the
	// update that applies the invalidation. It is called synchronously by the operator on
	// shutdown, outside the rendering loop.
	Finalize(gen int) *event.EventUpdate
}

// configRenderer is a generic interface for the rendering components that can generate components
// of the dataplane config.
type configRenderer interface {
	render(c *RenderContext, args ...any) (stnrapiv1.Config, error)
}

// clusterRenderer is the interface for the component that renders the dataplane cluster config of
// a route. It is separate from configRenderer because a cluster is rendered per route rather than
// per Gateway, so it takes the route instead of a RenderContext.
type clusterRenderer interface {
	renderCluster(ro store.Route) (*stnrapiv1.ClusterConfig, error)
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
	clusterRenderer                               clusterRenderer
	dataplaneGenerator                            resourceGenerator
	gen                                           int
	renderCh                                      chan event.Event
	operatorCh                                    chan<- event.Event
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
		clusterRenderer:    newClusterRenderer(cfg.Logger.WithName("cluster-renderer")),
		dataplaneGenerator: newDataplaneGenerator(cfg.Scheme),
		renderCh:           make(chan event.Event, 10),
		gen:                0,
		ProgressTracker:    config.NewProgressTracker(),
		log:                cfg.Logger.WithName("renderer"),
	}
	r.log.V(4).Info("Renderer thread created (**default** renderer)")
	return r
}

// Start runs the rendering loop until the context ends.
func (r *renderer) Start(ctx context.Context) error {
	r.ctx = ctx

	heartbeat := time.NewTicker(metrics.LoopHeartbeatInterval)
	defer heartbeat.Stop()
	metrics.RecordRendererHeartbeat()

	for {
		select {
		case <-heartbeat.C:
			metrics.RecordRendererHeartbeat()

		case e := <-r.renderCh:
			metrics.RecordRendererHeartbeat()
			if e.GetType() != event.EventTypeRender {
				r.log.Info("Renderer thread received unknown event", "event", e.String())
				continue
			}

			r.ProgressUpdate(1)
			start := time.Now()
			r.Render(e.(*event.EventRender))
			metrics.RenderDuration.Observe(time.Since(start).Seconds())
			metrics.RenderTotal.Inc()
			r.ProgressUpdate(-1)

		case <-ctx.Done():
			return nil
		}
	}
}

// GetRenderChannel returns the channel onn which the renderer listenens to rendering requests.
func (r *renderer) GetRenderChannel() chan event.Event {
	return r.renderCh
}

// SetOperatorChannel sets the channel on which the operator event dispatcher listens.
func (r *renderer) SetOperatorChannel(ch chan<- event.Event) {
	r.operatorCh = ch
}

// renderAdmin is a wrapper for adminRenderer.render()
func (r *renderer) renderAdmin(c *RenderContext) (*stnrapiv1.AdminConfig, error) {
	conf, err := r.adminRenderer.render(c)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrapiv1.AdminConfig), nil
}

// renderAuth is a wrapper for authRenderer.render()
func (r *renderer) renderAuth(c *RenderContext) (*stnrapiv1.AuthConfig, error) {
	conf, err := r.authRenderer.render(c)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrapiv1.AuthConfig), nil
}

// renderListener is a wrapper for listenerRenderer.render()
func (r *renderer) renderListener(c *RenderContext, l *gwapiv1.Listener, rs []store.Route, ap gwAddrPort, targetPorts map[string]int) (*stnrapiv1.ListenerConfig, error) {
	conf, err := r.listenerRenderer.render(c, l, rs, ap, targetPorts)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrapiv1.ListenerConfig), nil
}

// renderCluster is a wrapper for clusterRenderer.renderCluster()
func (r *renderer) renderCluster(ro store.Route) (*stnrapiv1.ClusterConfig, error) {
	return r.clusterRenderer.renderCluster(ro)
}

// generateDataplane is a wrapper for dataplaneGenerator.generate()
func (r *renderer) generateDataplane(c *RenderContext) (client.Object, error) {
	obj, err := r.dataplaneGenerator.generate(c)
	if err != nil {
		return nil, err
	}
	return obj, nil
}
