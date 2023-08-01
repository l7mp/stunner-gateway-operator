package renderer

import (
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// Render generates and sets a STUNner daemon configuration from the Gateway API running-config
func (r *Renderer) Render(e *event.EventRender) {
	r.gen += 1
	r.log.Info("rendering configuration", "generation", r.gen, "event", e.String())

	switch config.DataplaneMode {
	case config.DataplaneModeLegacy:
		r.renderGatewayClass(e)
	case config.DataplaneModeManaged:
		r.renderManagedGateways(e)
	default:
		r.log.Info(`unknown dataplane mode (must be either "managed" or "legacy")`)
		return
	}
}

// renderGatewayClass generates and sets a STUNner daemon configuration in the "legacy" dataplane mode.
func (r *Renderer) renderGatewayClass(e *event.EventRender) {
	r.log.Info("commencing dataplane render", "mode", "legacy")

	r.log.V(1).Info("obtaining gateway-class objects")
	gcs := r.getGatewayClasses()

	if len(gcs) == 0 {
		r.log.Info("no gateway-class objects found", "event", e.String())
		return
	}

	if len(gcs) > 1 {
		names := []string{}
		for _, gc := range gcs {
			names = append(names, fmt.Sprintf("%q", store.GetObjectKey(gc)))
		}

		r.log.Info("multiple gateway-class objects found: this is most probably UNINTENED - "+
			"the operator will attempt to render a configuration for each gateway-class but there "+
			"is no guarantee that this will work - this mode is UNSUPPORTED, "+
			"if unsure, remove one of the gateway-class objects", "names", strings.Join(names, ", "))
	}

	// render each GatewayClass: hopefully they won's step on each other's throat: we cannot
	// help if multiple GatewayClasses (or the GatewayConfigs thereof) set the rendering
	// pipeline to render into the same configmap, but at least we can prevent race conditions
	// by serializing update requests on the updaterChannel
	for _, gc := range gcs {
		r.log.Info("rendering configuration", "gateway-class", store.GetObjectKey(gc))
		c := NewRenderContext(e, r, gc)

		r.log.V(1).Info("obtaining gateway-config", "gateway-class", gc.GetName())
		var err error
		c.gwConf, err = r.getGatewayConfig4Class(c)
		if err != nil {
			r.log.Error(err, "error obtaining gateway-config",
				"gateway-class", gc.GetName())
			r.invalidateGatewayClass(c, err)
			continue
		}

		r.log.V(1).Info("finding gateways", "gateway-class", store.GetObjectKey(gc))
		gws := r.getGateways4Class(c)
		c.gws.ResetGateways(gws)

		// render for ALL gateways that correspond to this gateway-class
		if err := r.renderForGateways(c); err != nil {
			// an irreparable error happened, invalidate the config and set all related
			// object statuses to signal the error
			r.log.Error(err, "rendering", "gateway-class", store.GetObjectKey(gc))
			r.invalidateGatewayClass(c, err)
		}

		setGatewayClassStatusAccepted(gc, nil)

		// send the update back to the operator
		r.operatorCh <- c.update
	}
}

// renderManagedGateways generates and sets a STUNner daemon configuration for the "managed" dataplane mode.
func (r *Renderer) renderManagedGateways(e *event.EventRender) {
	r.log.Info("commencing full dataplane render", "mode", "managed")

	r.log.V(1).Info("obtaining gateway-class objects")
	gcs := r.getGatewayClasses()

	if len(gcs) == 0 {
		r.log.Info("no gateway-class objects found", "event", e.String())
		return
	}

	for _, gc := range gcs {
		r.log.Info("rendering configuration", "gateway-class", store.GetObjectKey(gc))

		r.log.V(1).Info("obtaining gateway-config", "gateway-class", gc.GetName())

		gcCtx := NewRenderContext(e, r, gc)
		gwConf, err := r.getGatewayConfig4Class(gcCtx)
		if err != nil {
			r.log.Error(err, "error obtaining gateway-config",
				"gateway-class", gc.GetName())
			r.invalidateGatewayClass(gcCtx, err)
			r.operatorCh <- gcCtx.update

			continue
		}
		gcCtx.gwConf = gwConf

		// don't even start rendering if Dataplane is not available
		if _, err := r.getDataplane(gcCtx); err != nil {
			r.log.Error(err, "error obtaining Dataplane",
				"gateway-class", store.GetObjectKey(gc),
				"gateway-config", store.GetObjectKey(gwConf),
			)
			r.invalidateGatewayClass(gcCtx, err)
			r.operatorCh <- gcCtx.update

			continue
		}

		for _, gw := range r.getGateways4Class(gcCtx) {
			gw := gw

			r.log.V(1).Info("rendering for gateway",
				"gateway-class", store.GetObjectKey(gc),
				"gateway", store.GetObjectKey(gw),
			)

			gwCtx := NewRenderContext(e, r, gc)
			gwCtx.gwConf = gcCtx.gwConf
			gwCtx.gws.ResetGateways([]*gwapiv1a2.Gateway{gw})

			// render for this gateway
			if err := r.renderForGateways(gwCtx); err != nil {
				r.log.Error(err, "rendering",
					"gateway-class", store.GetObjectKey(gc),
					"gateway", store.GetObjectKey(gw),
				)
				r.invalidateGateways(gwCtx, err)
				continue
			}
			gcCtx.Merge(gwCtx)
		}

		setGatewayClassStatusAccepted(gc, nil)

		r.operatorCh <- gcCtx.update
	}
}

// renderForGateways renders a configuration for a set of Gateways (c.gws)
func (r *Renderer) renderForGateways(c *RenderContext) error {
	log := r.log
	gc := c.gc

	conf := stnrconfv1a1.StunnerConfig{
		ApiVersion: stnrconfv1a1.ApiVersion,
	}

	targetName, targetNamespace := getTarget(c)

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

	conf.Listeners = []stnrconfv1a1.ListenerConfig{}
	for _, gw := range c.gws.GetAll() {
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

			lc, err := r.renderListener(gw, c.gwConf, &l, rs, ap)
			if err != nil {
				// all listener rendering errors are critical: prevent the
				// rendering of the listener config
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

		if !r.isRouteControlled(ro) {
			continue
		}

		initRouteStatus(ro)

		renderRoute := false
		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]

			parentOutContext := r.isParentOutContext(c.gws, ro, &p)
			parentAccept := r.isParentAcceptingRoute(ro, &p, gc.GetName())

			// at least one parent accepts the route: render it!
			renderRoute = renderRoute || (!parentOutContext && parentAccept)
		}

		rc, err := r.renderCluster(ro)
		if err != nil {
			if IsNonCritical(err) {
				log.Info("non-critical error rendering cluster", "route",
					ro.GetName(), "error", err.Error())
				// note error but otherwise ignore
				err = nil
			} else {
				log.Error(err, "fatal error rendering cluster", "route",
					ro.GetName())
			}
		}

		if renderRoute && err == nil && rc != nil {
			conf.Clusters = append(conf.Clusters, *rc)
		}

		// set status: we can do this only once we know whether (1) the parent accepted the
		// route and (2) the backend refs were successfully resolved
		for i := range ro.Spec.ParentRefs {
			p := ro.Spec.ParentRefs[i]

			// set className="" -> do not consider class of the gw for setting the status
			parentAccept := r.isParentAcceptingRoute(ro, &p, "")

			setRouteConditionStatus(ro, &p, config.ControllerName, parentAccept, err)
		}

		// schedule for update: note that we may process the same UDPRoute several times,
		// in the context of different Gateways: Upsert makes sure the last render will be
		// updated
		c.update.UpsertQueue.UDPRoutes.Upsert(ro)
	}

	// schedule for update
	c.update.UpsertQueue.GatewayClasses.Upsert(gc)

	log.Info("STUNner dataplane configuration ready", "generation", r.gen, "config",
		conf.String())

	// schedule for update
	cm, err := r.renderConfig(c, targetName, targetNamespace, &conf)
	if err != nil {
		return err
	}

	// fmt.Printf("%#v\n", cm)

	c.update.UpsertQueue.ConfigMaps.Upsert(cm)

	if config.DataplaneMode == config.DataplaneModeManaged {
		dp, err := r.createDeployment(c)
		if err != nil {
			return err
		}
		c.update.UpsertQueue.Deployments.Upsert(dp)
	}

	return nil
}

// invalidateGatewayClass invalidates an entire gateway-class, with all the gateways underneath
func (r *Renderer) invalidateGatewayClass(c *RenderContext, reason error) {
	log := r.log
	gc := c.gc

	log.Info("invalidating configuration", "gateway-class", store.GetObjectKey(gc),
		"reason", reason.Error())

	if config.DataplaneMode == config.DataplaneModeLegacy && c.gwConf == nil {
		// this is the killer case: we have most probably lost our gatewayconfig and we
		// don't know which stunner config to invalidate; for now warn, later eliminate
		// such cases by putting a finalizer/owner-ref to GatewayConfigs once we have
		// started using them
		log.Info("no gateway-config: active STUNNer configuration may remain stale",
			"gateway-class", gc.GetName())
	}

	setGatewayClassStatusAccepted(gc, reason)
	c.update.UpsertQueue.GatewayClasses.Upsert(gc)

	log.V(1).Info("invalidating all gateway objects in gateway-class",
		"gateway-class", gc.GetName())

	c.gws.ResetGateways(r.getGateways4Class(c))
	r.invalidateGateways(c, reason)
}

// invalidateGateways invalidates a set of Gateways
func (r *Renderer) invalidateGateways(c *RenderContext, reason error) {
	log := r.log
	gc := c.gc

	for _, gw := range c.gws.GetAll() {
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

			// skip if we are not responsible
			if r.isParentOutContext(c.gws, ro, &p) {
				continue
			}

			accepted := r.isParentAcceptingRoute(ro, &p, gc.GetName())

			// render a stale cluster so that we know the ResolvedRefs status
			_, err := r.renderCluster(ro)

			setRouteConditionStatus(ro, &p, config.ControllerName, accepted, err)
		}

		c.update.UpsertQueue.UDPRoutes.Upsert(ro)
	}

	// schedule for update
	if c.gwConf != nil {
		targetName, targetNamespace := getTarget(c)
		if targetName == "" {
			// we didn't find any ConfigMaps, give up
			log.V(2).Info("could not invalidate ConfigMap: Could not identify ConfigMap namespace/name",
				"gateway-class", store.GetObjectKey(gc))
			return
		}
		cm, err := r.renderConfig(c, targetName, targetNamespace, nil)
		if err != nil {
			log.Error(err, "error invalidating ConfigMap", "target",
				fmt.Sprintf("%s/%s", targetNamespace, targetName))
			if !IsNonCritical(err) && config.DataplaneMode == config.DataplaneModeManaged {
				// critical error: we could not render a valid configuration so
				// remove deployment
				gw := c.gws.GetFirst()
				if gw == nil {
					return
				}
				dp := config.DataplaneTemplate(gw)
				c.update.DeleteQueue.Deployments.Upsert(&dp)
			}
			return

		}

		if err := controllerutil.SetOwnerReference(c.gwConf, cm, r.scheme); err != nil {
			log.Error(err, "cannot set owner reference", "owner", store.GetObjectKey(gc),
				"reference", store.GetObjectKey(cm))
		}

		c.update.UpsertQueue.ConfigMaps.Upsert(cm)
	}
}

func getTarget(c *RenderContext) (string, string) {
	// gw-config.StunnerConfig may override this
	targetName, targetNamespace := "", ""
	switch config.DataplaneMode {
	case config.DataplaneModeLegacy:
		targetName = opdefault.DefaultConfigMapName
		if c.gwConf.Spec.StunnerConfig != nil {
			targetName = *c.gwConf.Spec.StunnerConfig
		}
		targetNamespace = c.gwConf.GetNamespace()
	case config.DataplaneModeManaged:
		// assume it exists
		gw := c.gws.GetFirst()
		if gw == nil {
			return "", ""
		}
		targetName = gw.GetName()
		targetNamespace = gw.GetNamespace()
	}

	return targetName, targetNamespace
}
