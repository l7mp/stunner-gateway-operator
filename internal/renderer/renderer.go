package renderer

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"

	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"

	// "github.com/l7mp/stunner-gateway-operator/internal/updater"

	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

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
						"event", e.String())
					continue
				}

				if e.GetType() != event.EventTypeRender {
					r.log.Info("renderer thread received unknown event",
						"event", e.String())
					continue
				}

				// prepare a new update event Render will populate
				u := event.NewEventUpdate()

				// config is returned in the update event ConfigMap store
				err := r.Render(e.(*event.EventRender), u)
				if err != nil {
					r.log.Error(err, "could not render STUNner configuration")
					continue
				}

				// send the update
				r.renderCh <- u

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

// Render generates and sets a STUNner daemon configuration from the Gateway API running-config
func (r *Renderer) Render(e *event.EventRender, u *event.EventUpdate) error {
	log := r.log
	log.Info("rendering configuration", "event", e.String())

	// gw-config.StunnerConfig may override this
	target := config.DefaultConfigMapName

	conf := stunnerconfv1alpha1.StunnerConfig{
		ApiVersion: stunnerconfv1alpha1.ApiVersion,
	}

	log.V(1).Info("obtaining GatewayClass")
	gc, err := r.getGatewayClass()
	if err != nil {
		return err
	}

	setGatewayClassStatusScheduled(gc, r.op.GetControllerName())

	log.V(1).Info("obtaining GatewayConfig", "GatewayClass", gc.GetName())
	gwConf, err := r.getGatewayConfig4Class(gc)
	if err != nil {
		return err
	}

	if gwConf.Spec.StunnerConfig != nil {
		target = *gwConf.Spec.StunnerConfig
	}

	log.V(1).Info("rendering admin config")
	admin, err := r.renderAdmin(gwConf)
	if err != nil {
		return err
	}
	conf.Admin = *admin

	log.V(1).Info("rendering auth config")
	auth, err := r.renderAuth(gwConf)
	if err != nil {
		return err
	}
	conf.Auth = *auth

	log.V(1).Info("finding Gateways")

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

			conf.Listeners = append(conf.Listeners, *lc)
			setListenerStatus(gw, &l, true, ready, len(rs))
		}

		setGatewayStatusReady(gw, r.op.GetControllerName())
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		u.Gateways.Upsert(gw)
	}

	log.V(1).Info("processing UDPRoutes")
	rs := r.op.GetUDPRoutes()
	for _, ro := range rs {
		log.V(2).Info("considering", "route", ro.GetName())

		renderRoute := false
		initRouteStatus(ro)

		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]

			accepted := r.isParentAcceptingRoute(ro, &p)

			// at least one parent accepts the route: render it!
			renderRoute = renderRoute || accepted

			setRouteConditionStatus(ro, &p, r.op.GetControllerName(), accepted)
		}

		if renderRoute == true {
			rc, err := r.renderCluster(ro)
			if err != nil {
				log.Info("error rendering configuration for route", "route",
					ro.GetName(), "error", err.Error())

				continue
			}

			conf.Clusters = append(conf.Clusters, *rc)
		}

		// schedule for update
		u.UDPRoutes.Upsert(ro)
	}

	setGatewayClassStatusReady(gc, r.op.GetControllerName())
	// schedule for update
	u.GatewayClasses.Upsert(gc)

	log.Info("STUNner dataplane configuration ready", "conf", fmt.Sprintf("%#v", conf))

	// fmt.Printf("target: %s, conf: %#v\n", target, conf)

	// schedule for update
	cm, err := r.renderStunnerConf2ConfigMap(gwConf.GetNamespace(), target, conf)
	if err != nil {
		return err
	}

	// set the GatewayClass as an owner
	// if mgr := r.op.GetManager(); mgr != nil {
	// 	// we are running in the test harness: no manager!
	// 	if err := controllerutil.SetOwnerReference(gc, cm, mgr.GetScheme()); err != nil {
	// 		log.Error(err, "cannot set owner reference on object (ignoring)", "owner",
	// 			store.GetObjectKey(gc), "configmap", store.GetObjectKey(cm))
	// 	}
	// }

	// do it without a scheme:
	// https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html
	kind := reflect.TypeOf(corev1.ConfigMap{}).Name()
	gvk := corev1.SchemeGroupVersion.WithKind(kind)
	controllerRef := metav1.NewControllerRef(gc, gvk)
	cm.SetOwnerReferences([]metav1.OwnerReference{*controllerRef})

	u.ConfigMaps.Upsert(cm)

	return nil
}

func (r *Renderer) renderStunnerConf2ConfigMap(ns, name string, conf stunnerconfv1alpha1.StunnerConfig) (*corev1.ConfigMap, error) {
	sc, err := json.Marshal(conf)
	if err != nil {
		return nil, err
	}

	immutable := true
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Immutable: &immutable,
		Data: map[string]string{
			config.DefaultStunnerdConfigfileName: string(sc),
		},
	}, nil
}
