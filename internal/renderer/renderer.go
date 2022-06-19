package renderer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"

	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

type RendererConfig struct {
	Logger logr.Logger
}

type Renderer struct {
	ctx      context.Context
	op       *operator.Operator
	renderCh chan event.Event
	log      logr.Logger
}

// NewRenderer creates a new Renderer
func NewRenderer(cfg RendererConfig) *Renderer {
	return &Renderer{
		renderCh: make(chan event.Event, 5),
		log:      cfg.Logger,
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
				if r.op == nil {
					r.log.Info("renderer thread uninitialized: operator unset",
						"event-type", e.Type.String(), "event",
						fmt.Sprintf("%#v", e))
					continue
				}

				err := r.ProcessEvent(e)
				if err != nil {
					r.log.Error(err, "could not process event", "event-type",
						e.Type.String(), "event", fmt.Sprintf("%#v", e))
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// SetOperator sets the operator associated with this renderer
func (r *Renderer) SetOperator(op *operator.Operator) {
	r.op = op
}

// GetRenderChannel returns the channel onn which the renderer listenens to rendering requests
func (r *Renderer) GetRenderChannel() chan event.Event {
	return r.renderCh
}

// ProcessEvent dispatches an event to the corresponding renderer
func (r *Renderer) ProcessEvent(e event.Event) error {
	switch e.Type {
	case event.EventTypeRender:
		_, err := r.Render(e)
		if err != nil {
			return err
		}
		return nil
	}

	return nil
}

// Render generates and sets a STUNner daemon configuration from the Gateway API running-config
func (r *Renderer) Render(e event.Event) (*stunnerconfv1alpha1.StunnerConfig, error) {
	log := r.log
	log.Info("rendering configuration", "event", e.String(), "origin", e.Origin, "reason",
		e.Reason)

	// gw-config.StunnerConfig may override this
	target := operator.DefaultConfigMapName

	log.V(2).Info("rendering stunner configuration")
	conf := stunnerconfv1alpha1.StunnerConfig{
		ApiVersion: stunnerconfv1alpha1.ApiVersion,
	}

	log.V(2).Info("obtaining GatewayClass")
	gc, err := r.getGatewayClass()
	if err != nil {
		return nil, err
	}

	// setStatusConditionScheduled(gc)

	log.V(2).Info("obtaining GatewayConfig", "GatewayClass", gc.GetName())
	gwConf, err := r.getGatewayConfig4Class(gc)
	if err != nil {
		return nil, err
	}

	if gwConf.Spec.StunnerConfig != nil {
		target = *gwConf.Spec.StunnerConfig
	}

	log.V(2).Info("rendering admin config")
	admin, err := r.renderAdmin(gwConf)
	if err != nil {
		return nil, err
	}
	conf.Admin = *admin

	log.V(2).Info("rendering auth config")
	auth, err := r.renderAuth(gwConf)
	if err != nil {
		return nil, err
	}
	conf.Auth = *auth

	log.V(2).Info("finding Gateways")

	acceptedRoutes := []routeGatewayPair{}
	for _, gw := range r.getGateways4Class(gc) {
		log.V(2).Info("considering", "gateway", gw.GetName())

		// this also re-inits listener statuses
		setGatewayStatusScheduled(gw, r.op.GetControllerName())

		log.V(3).Info("obtaining public address", "gateway", gw.GetName())
		var ready bool
		addr, err := r.getPublicAddrs4Gateway(gw)
		if err != nil {
			log.V(1).Info("cannot find public address", "gateway",
				gw.GetName(), "error", err.Error())
			ready = false
		} else {
			ready = true
		}

		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]
			log.V(3).Info("obtaining routes", "gateway", gw.GetName(), "listener",
				l.Name)
			rs := r.getUDPRoutes4Listener(gw, &l)

			lc, err := r.renderListener(gw, gwConf, &l, rs, addr)
			if err != nil {
				log.Info("error rendering configuration for listener", "gateway",
					gw.GetName(), "listener", l.Name, "error", err.Error())

				setListenerStatus(gw, &l, false, ready, 0)
				continue
			}

			for _, r := range rs {
				acceptedRoutes = append(acceptedRoutes, routeGatewayPair{
					route:    r,
					gateway:  gw,
					listener: &l,
				})
			}

			conf.Listeners = append(conf.Listeners, *lc)
			setListenerStatus(gw, &l, true, ready, len(rs))
		}

		setGatewayStatusReady(gw, r.op.GetControllerName())
		gw = pruneGatewayStatusConds(gw)
	}

	log.V(2).Info("processing UDPRoutes")
	rs := r.op.GetUDPRoutes()
	for _, ro := range rs {
		log.V(2).Info("considering", "route", ro.GetName())

		initRouteStatus(ro)
	}

	log.V(1).Info("stunner configuration ready", "conf", fmt.Sprintf("%#v", conf))

	fmt.Printf("target: %s, conf: %#v\n", target, conf)

	return &conf, nil
}
