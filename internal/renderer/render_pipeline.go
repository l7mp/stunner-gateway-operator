package renderer

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/updater"
	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// Render generates and sets a STUNner daemon configuration from the Gateway API running-config
func (r *Renderer) Render(e *event.EventRender) {
	log := r.log
	r.gen += 1
	log.Info("rendering configuration", "generation", r.gen, "event", e.String())

	log.V(1).Info("obtaining gateway-class objects")
	gcs := r.getGatewayClasses()

	if len(gcs) == 0 {
		log.Info("no gateway-class objects found", "event", e.String())
		return
	}

	if len(gcs) > 1 {
		names := []string{}
		for _, gc := range gcs {
			names = append(names, fmt.Sprintf("%q", store.GetObjectKey(gc)))
		}

		log.Info("multiple GatewayClass found %s: this is most probably UNINTENED -"+
			"the operator will attempt to render a configuration for all gateway-clases but there "+
			"is no guarantee that this will not result in an error - this mode is UNSUPPORTED, "+
			"if unsure, remove one of the gateway-classes!", strings.Join(names, ", "))
	}

	// render each GatewayClass: hopefully they won's step on each other's throat: we cannot
	// help if multiple GatewayClasses (or the GatewayConfigs thereof) set the rendering
	// pipeline to render into the same configmap, but at least we can prevent race conditions
	// by serializing update requests on the updaterChannel
	for _, gc := range gcs {
		u := event.NewEventUpdate()

		if err := r.renderGatewayClass(gc, u); err != nil {
			// an irreparable error happened, invalidate the config and set all related
			// object statuses to signal the error
			log.Error(err, "rendering", "gateway-class",
				store.GetObjectKey(gc))
			r.invalidateGatewayClass(gc, u, err)
		}

		// send the update back to the operator
		r.operatorCh <- u
	}
}

func (r *Renderer) renderGatewayClass(gc *gatewayv1alpha2.GatewayClass, u *event.EventUpdate) error {
	log := r.log
	log.Info("rendering configuration", "gateway-class", store.GetObjectKey(gc))

	setGatewayClassStatusScheduled(gc)

	// gw-config.StunnerConfig may override this
	target := config.DefaultConfigMapName

	conf := stunnerconfv1alpha1.StunnerConfig{
		ApiVersion: stunnerconfv1alpha1.ApiVersion,
	}

	setGatewayClassStatusReady(gc, nil)

	log.V(1).Info("obtaining gateway-config", "gateway-class", gc.GetName())
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

	log.V(1).Info("finding gateway objects")
	conf.Listeners = []stunnerconfv1alpha1.ListenerConfig{}
	for _, gw := range r.getGateways4Class(gc) {
		log.V(2).Info("considering", "gateway", gw.GetName())

		// this also re-inits listener statuses
		setGatewayStatusScheduled(gw, config.ControllerName)

		log.V(3).Info("obtaining public address", "gateway", gw.GetName())
		var ready bool
		addr, err := r.getPublicAddrs4Gateway(gw)
		if err != nil {
			log.Info("cannot find public address", "gateway", gw.GetName(), "error",
				err.Error())
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

		setGatewayStatusReady(gw, nil)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		u.Gateways.Upsert(gw)
	}

	log.V(1).Info("processing UDPRoutes")
	conf.Clusters = []stunnerconfv1alpha1.ClusterConfig{}
	rs := store.UDPRoutes.GetAll()
	for _, ro := range rs {
		log.V(2).Info("considering", "route", ro.GetName())

		renderRoute := false
		initRouteStatus(ro)

		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]

			accepted := r.isParentAcceptingRoute(ro, &p)

			// at least one parent accepts the route: render it!
			renderRoute = renderRoute || accepted

			setRouteConditionStatus(ro, &p, config.ControllerName, accepted)
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

	setGatewayClassStatusReady(gc, nil)
	// schedule for update
	u.GatewayClasses.Upsert(gc)

	log.Info("STUNner dataplane configuration ready", "generation", r.gen, "conf",
		fmt.Sprintf("%#v", conf))

	// fmt.Printf("target: %s, conf: %#v\n", target, conf)

	// schedule for update
	cm, err := r.renderConfigMap(gwConf.GetNamespace(), target, &conf)
	if err != nil {
		return err
	}

	// set the GatewayClass as an owner without using the scheme:
	// https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html
	kind := reflect.TypeOf(corev1.ConfigMap{}).Name()
	gvk := corev1.SchemeGroupVersion.WithKind(kind)
	controllerRef := metav1.NewControllerRef(gc, gvk)
	cm.SetOwnerReferences([]metav1.OwnerReference{*controllerRef})

	u.ConfigMaps.Upsert(cm)

	return nil
}

// this never reports errors: we cannot do about such errors anyway
func (r *Renderer) invalidateGatewayClass(gc *gatewayv1alpha2.GatewayClass, u *event.EventUpdate, reason error) {
	log := r.log
	log.Info("invalidating configuration", "gateway-class", store.GetObjectKey(gc),
		"reason", reason.Error())

	setGatewayClassStatusScheduled(gc)
	invalidateConf := true

	// gw-config.StunnerConfig may override this
	target := config.DefaultConfigMapName

	setGatewayClassStatusReady(gc, reason)
	u.GatewayClasses.Upsert(gc)

	log.V(1).Info("obtaining gateway-config", "gateway-class", gc.GetName())
	gwConf, err := r.getGatewayConfig4Class(gc)
	if err != nil {
		// this is the killer case: we have most probably lost our gatewayconfig and we
		// don't know which stunner config to invalidate; for now warn, later eliminate
		// such cases by putting a finalizer/owner-ref to GatewayConfigs once we have
		// started using them
		log.Info("cannot find the gateway-config: active STUNNer configuration may remain stale",
			"gateway-class", gc.GetName())
		invalidateConf = false
	} else {
		if gwConf.Spec.StunnerConfig != nil {
			target = *gwConf.Spec.StunnerConfig
		}
	}

	log.V(1).Info("finding gateway objects")
	for _, gw := range r.getGateways4Class(gc) {
		log.V(2).Info("considering", "gateway", gw.GetName())

		// this also re-inits listener statuses
		setGatewayStatusScheduled(gw, config.ControllerName)

		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]
			setListenerStatus(gw, &l, true, false, 0)
		}

		setGatewayStatusReady(gw, reason)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		u.Gateways.Upsert(gw)
	}

	log.V(1).Info("processing UDPRoutes")
	rs := store.UDPRoutes.GetAll()
	for _, ro := range rs {
		log.V(2).Info("considering", "route", ro.GetName())

		initRouteStatus(ro)

		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]
			accepted := r.isParentAcceptingRoute(ro, &p)
			setRouteConditionStatus(ro, &p, config.ControllerName, accepted)
		}

		u.UDPRoutes.Upsert(ro)
	}

	// fmt.Printf("target: %s, conf: %#v\n", target, conf)

	// schedule for update
	if invalidateConf == true {
		cm, err := r.renderConfigMap(gwConf.GetNamespace(), target, nil)
		if err != nil {
			log.Error(err, "error invalidating ConfigMap", "target", target)
			invalidateConf = false
		} else {
			// set the GatewayClass as an owner without using the scheme:
			// https://book.kubebuilder.io/cronjob-tutorial/writing-tests.html
			kind := reflect.TypeOf(corev1.ConfigMap{}).Name()
			gvk := corev1.SchemeGroupVersion.WithKind(kind)
			controllerRef := metav1.NewControllerRef(gc, gvk)
			cm.SetOwnerReferences([]metav1.OwnerReference{*controllerRef})

			u.ConfigMaps.Upsert(cm)
		}
	}
}

func (r *Renderer) renderConfigMap(ns, name string, conf *stunnerconfv1alpha1.StunnerConfig) (*corev1.ConfigMap, error) {
	s := ""

	if conf != nil {
		sc, err := json.Marshal(*conf)
		if err != nil {
			return nil, err
		} else {
			s = string(sc)
		}
	}

	immutable := true
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Immutable: &immutable,
		Data: map[string]string{
			config.DefaultStunnerdConfigfileName: s,
		},
	}, nil
}
