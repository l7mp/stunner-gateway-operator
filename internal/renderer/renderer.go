package renderer

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

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
	render(c *RenderContext, args ...any) (stnrapiv2.Config, error)
}

// listenerRenderer is the interface for the component that renders a Gateway listener into a
// dataplane listener.
type listenerRenderer interface {
	renderListener(c *RenderContext, l *gwapiv1.Listener, rs []store.Route, ap gwAddrPort, targetPorts map[string]int) (*stnrapiv2.ListenerConfig, error)
}

// serverRenderer is the interface for the component that renders the dataplane server a Gateway
// listener feeds.
type serverRenderer interface {
	renderServer(c *RenderContext, l *gwapiv1.Listener, rs []store.Route) (*stnrapiv2.ServerConfig, error)
}

// filterRenderer is the interface for the component that renders the filters of a server from the
// peers of the rendered clusters.
type filterRenderer interface {
	renderServerFilters(server *stnrapiv2.ServerConfig, peers map[string][]stnrapiv2.FilterRule) []stnrapiv2.FilterConfig
}

// clusterRenderer is the interface for the component that renders the dataplane cluster config of
// a route, plus the filter rules of the route, kept out of the cluster
// endpoints: an ALLOW rule per peer address block, with the port range of its backend if the
// backend sets one. A cluster is rendered per route, in the render context of the Gateways that
// carry the route; the default renderer uses only the route.
type clusterRenderer interface {
	renderCluster(c *RenderContext, ro store.Route) (*stnrapiv2.ClusterConfig, []stnrapiv2.FilterRule, error)
}

// gatewayRenderer is the interface for the component that renders what concerns a Gateway.
type gatewayRenderer interface {
	check(c *RenderContext, gw *gwapiv1.Gateway) error
	finalize(c *RenderContext, gw *gwapiv1.Gateway, conf *stnrapiv2.StunnerConfig) error
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
	ctx                         context.Context
	scheme                      *runtime.Scheme
	licmgr                      licensemgr.Manager
	adminRenderer, authRenderer configRenderer
	listenerRenderer            listenerRenderer
	serverRenderer              serverRenderer
	clusterRenderer             clusterRenderer
	filterRenderer              filterRenderer
	gatewayRenderer             gatewayRenderer
	dataplaneGenerator          resourceGenerator
	gen                         int
	renderCh                    chan event.Event
	operatorCh                  chan<- event.Event
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
		serverRenderer:     newServerRenderer(cfg.Logger.WithName("server-renderer")),
		clusterRenderer:    newClusterRenderer(cfg.Logger.WithName("cluster-renderer")),
		filterRenderer:     newFilterRenderer(cfg.Logger.WithName("filter-renderer")),
		gatewayRenderer:    newGatewayRenderer(),
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

// GetRenderChannel returns the channel on which the renderer listens to rendering requests.
func (r *renderer) GetRenderChannel() chan event.Event {
	return r.renderCh
}

// SetOperatorChannel sets the channel on which the operator event dispatcher listens.
func (r *renderer) SetOperatorChannel(ch chan<- event.Event) {
	r.operatorCh = ch
}

// renderAdmin is a wrapper for adminRenderer.render()
func (r *renderer) renderAdmin(c *RenderContext) (*stnrapiv2.AdminConfig, error) {
	conf, err := r.adminRenderer.render(c)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrapiv2.AdminConfig), nil
}

// renderAuth is a wrapper for authRenderer.render()
func (r *renderer) renderAuth(c *RenderContext) (*stnrapiv2.AuthConfig, error) {
	conf, err := r.authRenderer.render(c)
	if err != nil {
		return nil, err
	}
	return conf.(*stnrapiv2.AuthConfig), nil
}

// renderListener is a wrapper for listenerRenderer.renderListener()
func (r *renderer) renderListener(c *RenderContext, l *gwapiv1.Listener, rs []store.Route, ap gwAddrPort, targetPorts map[string]int) (*stnrapiv2.ListenerConfig, error) {
	return r.listenerRenderer.renderListener(c, l, rs, ap, targetPorts)
}

// renderServer is a wrapper for serverRenderer.renderServer()
func (r *renderer) renderServer(c *RenderContext, l *gwapiv1.Listener, rs []store.Route) (*stnrapiv2.ServerConfig, error) {
	return r.serverRenderer.renderServer(c, l, rs)
}

// renderCluster is a wrapper for clusterRenderer.renderCluster()
func (r *renderer) renderCluster(c *RenderContext, ro store.Route) (*stnrapiv2.ClusterConfig, []stnrapiv2.FilterRule, error) {
	return r.clusterRenderer.renderCluster(c, ro)
}

// renderServerFilters is a wrapper for filterRenderer.renderServerFilters()
func (r *renderer) renderServerFilters(server *stnrapiv2.ServerConfig, peers map[string][]stnrapiv2.FilterRule) []stnrapiv2.FilterConfig {
	return r.filterRenderer.renderServerFilters(server, peers)
}

// generateDataplane is a wrapper for dataplaneGenerator.generate()
func (r *renderer) generateDataplane(c *RenderContext) (client.Object, error) {
	return r.dataplaneGenerator.generate(c)
}
