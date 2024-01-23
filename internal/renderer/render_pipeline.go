package renderer

import (
	"errors"
	"fmt"
	"strings"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

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
		r.operatorCh <- c.update.DeepCopy()
	}
}

// renderManagedGateways generates and sets a STUNner daemon configuration for the "managed" dataplane mode.
func (r *Renderer) renderManagedGateways(e *event.EventRender) {
	r.log.Info("commencing dataplane render", "mode", "managed")

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
			r.operatorCh <- gcCtx.update.DeepCopy()

			continue
		}
		gcCtx.gwConf = gwConf

		// don't even start rendering if Dataplane is not available
		dp, err := getDataplane(gcCtx)
		if err != nil {
			r.log.Error(err, "error obtaining Dataplane",
				"gateway-class", store.GetObjectKey(gc),
				"gateway-config", store.GetObjectKey(gwConf),
			)
			r.invalidateGatewayClass(gcCtx, err)
			r.operatorCh <- gcCtx.update.DeepCopy()

			continue
		}
		gcCtx.dp = dp

		for _, gw := range r.getGateways4Class(gcCtx) {
			gw := gw

			r.log.V(1).Info("rendering for gateway",
				"gateway-class", store.GetObjectKey(gc),
				"gateway", store.GetObjectKey(gw),
			)

			gwCtx := NewRenderContext(e, r, gc)
			gwCtx.gwConf = gcCtx.gwConf
			gwCtx.dp = gcCtx.dp
			gwCtx.gws.ResetGateways([]*gwapiv1.Gateway{gw})

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

		r.operatorCh <- gcCtx.update.DeepCopy()
	}
}

// renderForGateways renders a configuration for a set of Gateways (c.gws)
func (r *Renderer) renderForGateways(c *RenderContext) error {
	log := r.log
	gc := c.gc

	conf := stnrconfv1.StunnerConfig{
		ApiVersion: stnrconfv1.ApiVersion,
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

	conf.Listeners = []stnrconfv1.ListenerConfig{}
	for _, gw := range c.gws.GetAll() {
		log.V(2).Info("considering", "gateway", store.GetObjectKey(gw), "listener-num",
			len(gw.Spec.Listeners))

		initGatewayStatus(gw, config.ControllerName)

		log.V(3).Info("obtaining public address", "gateway", store.GetObjectKey(gw))
		pubGwAddrs, err := r.getPublicAddr(gw)
		if err != nil {
			log.V(1).Info("cannot find public address", "gateway", store.GetObjectKey(gw),
				"error", err.Error())
		}

		// recreate the LoadBalancer service, otherwise a changed
		// GatewayConfig.Spec.LoadBalancerServiceAnnotation or Gateway annotation may not
		// be reflected back to the service
		if s := r.createLbService4Gateway(c, gw); s != nil {
			log.Info("creating public service for gateway", "service",
				store.GetObjectKey(s), "gateway", store.GetObjectKey(gw),
				"service", store.DumpObject(s))

			c.update.UpsertQueue.Services.Upsert(s)
		}

		udpPorts := make(map[int]bool)
		tcpPorts := make(map[int]bool)
		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]

			log.V(3).Info("obtaining routes", "gateway", store.GetObjectKey(gw), "listener",
				l.Name)
			rs := r.getUDPRoutes4Listener(gw, &l)

			if isListenerConflicted(&l, udpPorts, tcpPorts) {
				log.Info("listener protocol/port conflict", "gateway", store.GetObjectKey(gw),
					"listener", l.Name)
				setListenerStatus(gw, &l, NewNonCriticalError(PortUnavailable), true, len(rs))
				continue
			}

			lc, err := r.renderListener(gw, c.gwConf, &l, rs, pubGwAddrs[j])
			if err != nil {
				// all listener rendering errors are critical: prevent the
				// rendering of the listener config
				log.Info("error rendering configuration for listener", "gateway",
					store.GetObjectKey(gw), "listener", l.Name, "error", err.Error())

				setListenerStatus(gw, &l, err, false, 0)
				continue
			}

			conf.Listeners = append(conf.Listeners, *lc)
			setListenerStatus(gw, &l, nil, false, len(rs))
		}

		setGatewayStatusProgrammed(gw, nil, pubGwAddrs)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		c.update.UpsertQueue.Gateways.Upsert(gw)
	}

	log.V(1).Info("processing UDPRoutes")
	conf.Clusters = []stnrconfv1.ClusterConfig{}
	for _, ro := range r.allUDPRoutes() {
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
		criticalErr := err
		if err != nil {
			if IsNonCritical(err) {
				log.Info("non-critical error rendering cluster", "route",
					store.GetObjectKey(ro), "error", err.Error())
				// note error but otherwise ignore
				criticalErr = nil
			} else {
				log.Error(err, "fatal error rendering cluster", "route",
					ro.GetName())
			}
		}

		if renderRoute && criticalErr == nil && rc != nil {
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
		if isRouteV1A2(ro) {
			c.update.UpsertQueue.UDPRoutesV1A2.Upsert(ro)
		} else {
			c.update.UpsertQueue.UDPRoutes.Upsert(ro)
		}
	}
	r.invalidateMaskedRoutes(c)
	r.log.Info(c.update.String())

	// schedule for update
	c.update.UpsertQueue.GatewayClasses.Upsert(gc)

	if config.DataplaneMode == config.DataplaneModeManaged {
		// config name is the name of the gateway
		gw := c.gws.GetFirst()
		if gw != nil {
			conf.Admin.Name = store.GetObjectKey(gw)

			// update cds server
			c.update.ConfigQueue = append(c.update.ConfigQueue, &conf)

			// create deployment
			dp, err := r.createDeployment(c)
			if err != nil {
				return err
			}
			c.update.UpsertQueue.Deployments.Upsert(dp)
			log.Info("STUNner dataplane Deployment ready", "generation", r.gen,
				"deployment", store.DumpObject(dp))
		}
	}

	log.Info("STUNner dataplane configuration ready", "generation", r.gen, "config",
		conf.String())

	cm, err := r.renderConfig(c, targetName, targetNamespace, &conf)
	if err != nil {
		return err
	}
	c.update.UpsertQueue.ConfigMaps.Upsert(cm)

	return nil
}

// invalidateGatewayClass invalidates an entire gateway-class, with all the gateways underneath
func (r *Renderer) invalidateGatewayClass(c *RenderContext, reason error) {
	log := r.log
	gc := c.gc

	log.Info("invalidating configuration", "gateway-class", store.GetObjectKey(gc),
		"reason", reason.Error())

	if config.DataplaneMode == config.DataplaneModeLegacy {
		if c.gwConf != nil {
			// remove the configmap
			targetNamespace := c.gwConf.GetNamespace()
			targetName := opdefault.DefaultConfigMapName
			if cm, err := r.renderConfig(c, targetName, targetNamespace, nil); err != nil {
				log.Error(err, "error invalidating ConfigMap", "target",
					fmt.Sprintf("%s/%s", targetNamespace, targetName))
			} else {
				c.update.UpsertQueue.ConfigMaps.Upsert(cm)
			}
		} else {
			// this is the killer case: we have most probably lost our gatewayconfig and we
			// don't know which stunner config to invalidate; for now warn, later eliminate
			// such cases by putting a finalizer/owner-ref to GatewayConfigs once we have
			// started using them
			log.Info("no gateway-config: active STUNNer configuration may remain stale",
				"gateway-class", gc.GetName())
		}
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
		log.V(2).Info("considering", "gateway", store.GetObjectKey(gw), "listener-num", len(gw.Spec.Listeners))

		// this also re-inits listener statuses
		initGatewayStatus(gw, config.ControllerName)

		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]
			setListenerStatus(gw, &l, errors.New("invalid"), false, 0)
		}

		setGatewayStatusProgrammed(gw, reason, nil)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		c.update.UpsertQueue.Gateways.Upsert(gw)

		// delete dataplane configmaps and deployments for invalidated gateways
		// we do not update the client via CDS: the deployment is going away anyway
		if config.DataplaneMode == config.DataplaneModeManaged {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gw.GetName(),
					Namespace: gw.GetNamespace(),
				},
			}
			c.update.DeleteQueue.ConfigMaps.Upsert(cm)
			log.V(2).Info("deleting dataplane ConfigMap", "generation", r.gen,
				"deployment", store.DumpObject(cm))

			dp := &appv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gw.GetName(),
					Namespace: gw.GetNamespace(),
				},
			}
			c.update.DeleteQueue.Deployments.Upsert(dp)
			log.V(2).Info("deleting dataplane Deployment", "generation", r.gen,
				"deployment", store.DumpObject(dp))
		}
	}

	log.V(1).Info("processing UDPRoutes")
	for _, ro := range r.allUDPRoutes() {
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

		if isRouteV1A2(ro) {
			c.update.UpsertQueue.UDPRoutesV1A2.Upsert(ro)
		} else {
			c.update.UpsertQueue.UDPRoutes.Upsert(ro)
		}
	}
}

func getTarget(c *RenderContext) (string, string) {
	// gw-config.StunnerConfig may override this
	targetName, targetNamespace := "", ""
	switch config.DataplaneMode {
	case config.DataplaneModeLegacy:
		targetName = opdefault.DefaultConfigMapName
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
