package renderer

import (
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/client"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// corev1 "k8s.io/api/core/v1"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/updater"
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

		log.Info("multiple gateway-class objects found: this is most probably UNINTENED - "+
			"the operator will attempt to render a configuration for each gateway-class but there "+
			"is no guarantee that this will work - this mode is UNSUPPORTED, "+
			"if unsure, remove one of the gateway-class objects", "names", strings.Join(names, ", "))
	}

	// render each GatewayClass: hopefully they won's step on each other's throat: we cannot
	// help if multiple GatewayClasses (or the GatewayConfigs thereof) set the rendering
	// pipeline to render into the same configmap, but at least we can prevent race conditions
	// by serializing update requests on the updaterChannel
	for _, gc := range gcs {
		c := NewRenderContext(e, r, gc)
		if err := r.renderGatewayClass(c); err != nil {
			// an irreparable error happened, invalidate the config and set all related
			// object statuses to signal the error
			log.Error(err, "rendering", "gateway-class", store.GetObjectKey(gc))
			r.invalidateGatewayClass(c, err)
		}

		// send the update back to the operator
		r.operatorCh <- c.update
	}
}

func (r *Renderer) renderGatewayClass(c *RenderContext) error {
	log := r.log
	gc := c.gc
	log.Info("rendering configuration", "gateway-class", store.GetObjectKey(gc))

	// gw-config.StunnerConfig may override this
	target := opdefault.DefaultConfigMapName

	conf := stnrconfv1a1.StunnerConfig{
		ApiVersion: stnrconfv1a1.ApiVersion,
	}

	log.V(1).Info("obtaining gateway-config", "gateway-class", gc.GetName())
	gwConf, err := r.getGatewayConfig4Class(c)
	if err != nil {
		return err
	}

	c.gwConf = gwConf
	setGatewayClassStatusAccepted(gc, nil)

	if gwConf.Spec.StunnerConfig != nil {
		target = *gwConf.Spec.StunnerConfig
	}

	log.V(1).Info("rendering admin config")
	admin, err := r.renderAdmin(c)
	if err != nil {
		return err
	}
	conf.Admin = *admin

	log.V(1).Info("rendering auth config")
	auth, err := r.renderAuth(c)
	if err != nil {
		return err
	}
	conf.Auth = *auth

	// all errors from this point are non-critical

	log.V(1).Info("finding gateway objects")
	conf.Listeners = []stnrconfv1a1.ListenerConfig{}
	for _, gw := range r.getGateways4Class(c) {
		log.V(2).Info("considering", "gateway", gw.GetName(), "listener-num", len(gw.Spec.Listeners))

		// this also re-inits listener statuses
		initGatewayStatus(gw, config.ControllerName)

		log.V(3).Info("obtaining public address", "gateway", gw.GetName())
		var ready bool
		ap, err := r.getPublicAddrPort4Gateway(gw)
		if err != nil {
			log.V(1).Info("cannot find public address", "gateway", gw.GetName(),
				"error", err.Error())
			ready = false
		} else if ap == nil {
			log.V(1).Info("cannot find public address", "gateway", gw.GetName())
			ready = false
		} else if ap.addr == "" {
			log.Info("public service found but no ExternalIP is available for service: " +
				"this is most probably caused by a fallback to a NodePort access service " +
				"but no nodes seem to be having a valid external IP address. Hint: " +
				"enable LoadBalancer services in Kubernetes")
			ready = false
		} else {
			ready = true
		}

		// recreate the LoadBalancer service, otherwise a changed
		// GatewayConfig.Spec.LoadBalancerServiceAnnotation or Gateway annotation may not
		// be reflected back to the service
		if s := createLbService4Gateway(c, gw); s != nil {
			if err := controllerutil.SetOwnerReference(gw, s, r.scheme); err != nil {
				r.log.Error(err, "cannot set owner reference", "owner",
					store.GetObjectKey(gw), "reference",
					store.GetObjectKey(s))
			}

			log.Info("creating public service for gateway", "name",
				store.GetObjectKey(s), "gateway", gw.GetName(), "service",
				store.DumpObject(s))

			c.update.UpsertQueue.Services.Upsert(s)
		}

		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]
			log.V(3).Info("obtaining routes", "gateway", gw.GetName(), "listener",
				l.Name)
			rs := r.getUDPRoutes4Listener(gw, &l)

			lc, err := r.renderListener(gw, gwConf, &l, rs, ap)
			if err != nil {
				log.Info("error rendering configuration for listener", "gateway",
					gw.GetName(), "listener", l.Name, "error", err.Error())

				setListenerStatus(gw, &l, false, ready, 0)
				continue
			}

			conf.Listeners = append(conf.Listeners, *lc)
			setListenerStatus(gw, &l, true, ready, len(rs))
		}

		setGatewayStatusProgrammed(gw, nil)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		c.update.UpsertQueue.Gateways.Upsert(gw)
	}

	log.V(1).Info("processing UDPRoutes")
	conf.Clusters = []stnrconfv1a1.ClusterConfig{}
	rs := store.UDPRoutes.GetAll()
	for _, ro := range rs {
		log.V(2).Info("considering", "route", ro.GetName())

		renderRoute := false
		parentAccept := make([]bool, len(ro.Spec.ParentRefs))

		initRouteStatus(ro)

		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]

			parentAccept[i] = r.isParentAcceptingRoute(ro, &p, gc.GetName())

			// at least one parent accepts the route: render it!
			renderRoute = renderRoute || parentAccept[i]
		}

		var backendErr error
		if renderRoute {
			rc, err := r.renderCluster(ro)
			if err != nil {
				backendErr = err
				if IsNonCritical(err) {
					log.Info("non-critical error rendering cluster", "route",
						ro.GetName(), "error", err.Error())
				} else {
					log.Info("fatal error rendering cluster", "route",
						ro.GetName(), "error", err.Error())
					continue
				}
			}

			conf.Clusters = append(conf.Clusters, *rc)
		}

		// set status: we can do this only once we know whether (1) the parent accepted the
		// route and (2) the backend refs were successfully resolved
		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]
			setRouteConditionStatus(ro, &p, config.ControllerName, parentAccept[i],
				backendErr)
		}

		// schedule for update
		c.update.UpsertQueue.UDPRoutes.Upsert(ro)
	}

	// schedule for update
	c.update.UpsertQueue.GatewayClasses.Upsert(gc)

	log.Info("STUNner dataplane configuration ready", "generation", r.gen, "config",
		conf.String())

	// schedule for update
	cm, err := r.renderConfig(c, target, &conf)
	if err != nil {
		return err
	}

	// fmt.Printf("%#v\n", cm)

	c.update.UpsertQueue.ConfigMaps.Upsert(cm)

	return nil
}

// this never reports errors: we cannot do about such errors anyway
func (r *Renderer) invalidateGatewayClass(c *RenderContext, reason error) {
	log := r.log
	gc := c.gc
	log.Info("invalidating configuration", "gateway-class", store.GetObjectKey(gc),
		"reason", reason.Error())
	invalidateConf := true

	// gw-config.StunnerConfig may override this
	target := opdefault.DefaultConfigMapName

	log.V(1).Info("obtaining gateway-config", "gateway-class", gc.GetName())
	gwConf, err := r.getGatewayConfig4Class(c)
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

	setGatewayClassStatusAccepted(gc, err)
	c.update.UpsertQueue.GatewayClasses.Upsert(gc)

	log.V(1).Info("finding gateway objects")
	for _, gw := range r.getGateways4Class(c) {
		log.V(2).Info("considering", "gateway", gw.GetName(), "listener-num", len(gw.Spec.Listeners))

		// this also re-inits listener statuses
		initGatewayStatus(gw, config.ControllerName)

		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]
			setListenerStatus(gw, &l, true, false, 0)
		}

		setGatewayStatusProgrammed(gw, reason)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		c.update.UpsertQueue.Gateways.Upsert(gw)
	}

	log.V(1).Info("processing UDPRoutes")
	rs := store.UDPRoutes.GetAll()
	for _, ro := range rs {
		log.V(2).Info("considering", "route", ro.GetName())

		initRouteStatus(ro)

		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]
			accepted := r.isParentAcceptingRoute(ro, &p, gc.GetName())

			// TODO: not sure what to set here so we set ResolvedRefs to
			// BackendNotFound
			setRouteConditionStatus(ro, &p, config.ControllerName, accepted,
				NewNonCriticalError(ServiceNotFound))
		}

		c.update.UpsertQueue.UDPRoutes.Upsert(ro)
	}

	// fmt.Printf("target: %s, conf: %#v\n", target, conf)

	// schedule for update
	if invalidateConf {
		cm, err := r.renderConfig(c, target, nil)
		if err != nil {
			log.Error(err, "error invalidating ConfigMap", "target", target)
			return
		}

		if err := controllerutil.SetOwnerReference(gwConf, cm, r.scheme); err != nil {
			log.Error(err, "cannot set owner reference", "owner", store.GetObjectKey(gc),
				"reference", store.GetObjectKey(cm))
		}

		c.update.UpsertQueue.ConfigMaps.Upsert(cm)
	}
}
