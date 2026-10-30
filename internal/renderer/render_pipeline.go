package renderer

import (
	"errors"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/config"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/event"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
)

// Render generates and sets a STUNner daemon configuration from the Gateway API running-config
func (r *renderer) Render(e *event.EventRender) {
	r.gen = e.Generation
	r.log.Info("Rendering configuration", "generation", r.gen, "event", e.String())

	r.renderManagedGateways(e)
}

// Finalize performs the finalization sequence:
// - set all managed Kubernetes statuses to invalid
// - remove managed dataplanes and services
// It returns the update that applies the invalidation, or nil when there is nothing to do.
func (r *renderer) Finalize(gen int) *event.EventUpdate {
	r.gen = gen

	return r.finalizeManagedGateways()
}

// renderManagedGateways generates and sets a STUNner daemon configuration for the "managed" dataplane mode.
func (r *renderer) renderManagedGateways(e *event.EventRender) {
	r.log.Info("Starting dataplane render", "mode", "managed")

	pipelineCtx := NewRenderContext(r, nil)

	r.log.V(1).Info("Obtaining gateway-class objects")
	gcs := r.getGatewayClasses()
	if len(gcs) == 0 {
		r.log.Info("No gateway-class objects found", "event", e.String())
		return
	}

	for _, gc := range gcs {
		r.log.Info("Rendering configuration", "gateway-class", store.GetObjectKey(gc))

		r.log.V(1).Info("Obtaining gateway-config", "gateway-class", gc.GetName())

		gcCtx := NewRenderContext(r, gc)
		gwConf, err := r.getGatewayConfig4Class(gcCtx)
		if err != nil {
			r.log.Error(err, "Error obtaining gateway-config", "gateway-class", gc.GetName())
			r.invalidateGatewayClass(gcCtx, err)
			pipelineCtx.Merge(gcCtx)
			continue
		}
		gcCtx.gwConf = gwConf

		// don't even start rendering if Dataplane is not available
		dp, err := getDataplane(gcCtx)
		if err != nil {
			r.log.Error(err, "Error obtaining Dataplane",
				"gateway-class", store.GetObjectKey(gc),
				"gateway-config", store.GetObjectKey(gwConf),
			)
			r.invalidateGatewayClass(gcCtx, err)
			pipelineCtx.Merge(gcCtx)
			continue
		}
		gcCtx.dp = dp

		for _, gw := range r.getGateways4Class(gcCtx) {
			r.log.V(1).Info("Rendering for gateway",
				"gateway-class", store.GetObjectKey(gc),
				"gateway", store.GetObjectKey(gw),
			)

			gwCtx := NewRenderContext(r, gc)
			gwCtx.gwConf = gcCtx.gwConf
			gwCtx.dp = gcCtx.dp
			gwCtx.gws.ResetGateways([]*gwapiv1.Gateway{gw})

			// render for this gateway
			if err := r.renderForGateways(gwCtx); err != nil {
				r.log.Error(err, "Rendering", "gateway-class", store.GetObjectKey(gc),
					"gateway", store.GetObjectKey(gw),
				)
				r.invalidateGateways(gwCtx, err)
				gcCtx.Merge(gwCtx)
				continue
			}
			gcCtx.Merge(gwCtx)
		}

		setGatewayClassStatusAccepted(gc, nil)
		gcCtx.update.UpsertQueue.GatewayClasses.Upsert(gc.DeepCopy())

		pipelineCtx.Merge(gcCtx)
	}

	u := pipelineCtx.update.DeepCopy()

	// updates must be acknowledged to the operator by the updater
	u.SetRequestAck(true)

	event.Send(r.ctx, r.operatorCh, u)
}

// finalizeManagedGateways invalidates all managed resources and returns the update that applies
// the invalidation.
func (r *renderer) finalizeManagedGateways() *event.EventUpdate {
	r.log.Info("Starting finalization", "mode", "managed")

	pipelineCtx := NewRenderContext(r, nil)

	r.log.V(1).Info("Obtaining gateway-class objects")
	gcs := r.getGatewayClasses()
	if len(gcs) == 0 {
		r.log.Info("No gateway-class objects found", "generation", r.gen)
		return nil
	}

	for _, gc := range gcs {
		r.log.Info("Invalidating statuses", "gateway-class", store.GetObjectKey(gc))

		gcCtx := NewRenderContext(r, gc)

		// generate a fake error
		err := errors.New("Operator unavailable after graceful shutdown")

		// invalidate class and all the associated gateways
		r.invalidateGatewayClass(gcCtx, err)
		pipelineCtx.Merge(gcCtx)
	}

	return pipelineCtx.update.DeepCopy()
}

// renderForGateways renders a configuration for a set of Gateways (c.gws)
func (r *renderer) renderForGateways(c *RenderContext) error {
	log := r.log
	gc := c.gc

	conf := stnrapiv2.StunnerConfig{
		ApiVersion: stnrapiv2.ApiVersion,
	}

	log.V(1).Info("Rendering admin config")
	admin, err := r.renderAdmin(c)
	if err != nil {
		return err
	}
	conf.Admin = *admin

	log.V(1).Info("Rendering auth config")
	auth, err := r.renderAuth(c)
	if err != nil {
		return err
	}
	conf.Auths = []stnrapiv2.AuthConfig{*auth}

	conf.Listeners = []stnrapiv2.ListenerConfig{}
	conf.Servers = []stnrapiv2.ServerConfig{}
	for _, gw := range c.gws.GetAll() {
		log.V(2).Info("Considering", "gateway", store.GetObjectKey(gw), "listener-num",
			len(gw.Spec.Listeners))

		if err := r.gatewayRenderer.check(c, gw); err != nil {
			return err
		}

		// create a private shallow copy of the render context for the listener renderer
		// this is not safe for modifying the render context
		gwCtx := *c
		gwCtx.gws = store.NewGatewayStore()
		gwCtx.gws.ResetGateways([]*gwapiv1.Gateway{gw})

		initGatewayStatus(gw, nil)

		log.V(3).Info("Obtaining public address", "gateway", store.GetObjectKey(gw))
		pubGwAddrs, err := r.getPublicAddr(gw)
		if err != nil {
			log.V(1).Info("Cannot find public address", "gateway", store.GetObjectKey(gw),
				"error", err.Error())
		}

		// recreate the LoadBalancer service, otherwise a changed
		// GatewayConfig.Spec.LoadBalancerServiceAnnotation or Gateway annotation may not
		// be reflected back to the service
		targetPorts := map[string]int{} // when the user selects a particular target port
		if s, ports := r.createLbService4Gateway(c, gw); s != nil {
			log.Info("Creating public service for gateway", "service",
				store.GetObjectKey(s), "gateway", store.GetObjectKey(gw),
				"service", store.DumpObject(s))

			c.update.UpsertQueue.Services.Upsert(s.DeepCopy())
			targetPorts = ports
		}

		udpPorts := make(map[int]bool)
		tcpPorts := make(map[int]bool)
		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]

			log.V(3).Info("Obtaining routes", "gateway", store.GetObjectKey(gw), "listener",
				l.Name)
			rs := r.getRoutes4Listener(gw, &l)

			if isListenerConflicted(&l, udpPorts, tcpPorts) {
				log.Info("Listener protocol/port conflict", "gateway", store.GetObjectKey(gw),
					"listener", l.Name)
				setListenerStatus(gw, &l, NewNonCriticalError(PortUnavailable), true, len(rs))
				continue
			}

			// the Gateway may remap the listener's target port from an annotation:
			// this is indicated in targetPorts
			lc, err := r.renderListener(&gwCtx, &l, rs, pubGwAddrs[j], targetPorts)
			if err != nil {
				// all listener rendering errors are critical: prevent the
				// rendering of the listener config
				log.Info("Error rendering configuration for listener", "gateway",
					store.GetObjectKey(gw), "listener", l.Name, "error", err.Error())

				setListenerStatus(gw, &l, err, false, 0)
				continue
			}

			sc, err := r.renderServer(&gwCtx, &l, rs)
			if err != nil {
				log.Info("Error rendering server configuration for listener", "gateway",
					store.GetObjectKey(gw), "listener", l.Name, "error", err.Error())
				setListenerStatus(gw, &l, err, false, 0)
				continue
			}

			conf.Listeners = append(conf.Listeners, *lc)
			conf.Servers = append(conf.Servers, *sc)
			setListenerStatus(gw, &l, nil, false, len(rs))
		}

		for _, l := range r.gatewayRenderer.sections(gw) {
			sc, err := r.renderServer(&gwCtx, &l, r.getRoutes4Listener(gw, &l))
			if err != nil {
				log.Info("Error rendering server configuration for section", "gateway",
					store.GetObjectKey(gw), "section", l.Name, "error", err.Error())
				continue
			}
			conf.Servers = append(conf.Servers, *sc)
		}

		setGatewayStatusProgrammed(gw, nil, pubGwAddrs)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		c.update.UpsertQueue.Gateways.Upsert(gw.DeepCopy())
	}

	log.V(1).Info("Processing routes")
	conf.Clusters = []stnrapiv2.ClusterConfig{}
	conf.Filters = []stnrapiv2.FilterConfig{}
	peers := map[string][]stnrapiv2.FilterRule{} // the peers of the rendered clusters
	for _, ro := range r.allRoutes() {
		log.V(2).Info("Considering", "route", ro.GetName())

		if !r.isRouteControlled(ro) {
			continue
		}

		initRouteStatus(ro)

		renderRoute := false
		parents := ro.GetParentRefs()
		for i := range parents {
			p := parents[i]

			parentOutContext := r.isParentOutContext(c.gws, ro, &p)
			parentExists, parentAccept := r.isParentAcceptingRoute(ro, &p, gc.GetName())

			// at least one existing parent accepts the route: render it!
			renderRoute = renderRoute || (!parentOutContext && parentExists && parentAccept)
		}

		// a critical error renders no cluster, a non-critical one only goes to the status
		rc, rcPeers, err := r.renderCluster(c, ro)
		if err != nil {
			log.Info("Error rendering cluster", "route", store.GetObjectKey(ro), "critical",
				IsCritical(err), "error", err.Error())
		}
		if renderRoute && rc != nil {
			conf.Clusters = append(conf.Clusters, *rc)
			peers[rc.Name] = rcPeers
			// the authentication policy of the tunnel of a TURNRoute
			if auth, _ := renderTunnelAuth(ro); auth != nil {
				conf.Auths = append(conf.Auths, *auth)
			}
		}

		// set status: we can do this only once we know whether (1) the parent accepted the
		// route and (2) the backend refs were successfully resolved
		for i := range parents {
			p := parents[i]

			// set className="" -> do not consider class of the gw for setting the status
			parentExists, parentAccept := r.isParentAcceptingRoute(ro, &p, "")
			setRouteConditionStatus(ro, &p, config.ControllerName, parentExists, parentAccept, err)
		}

		queueRouteStatusUpdate(c, ro)
	}
	r.invalidateMaskedRoutes(c)

	if gw := c.gws.GetFirst(); gw != nil {
		if err := r.gatewayRenderer.finalize(c, gw, &conf); err != nil {
			return err
		}
	}

	// the filters of the servers as they stand once the Gateway is finalized
	log.V(1).Info("Rendering filters")
	for i := range conf.Servers {
		s := &conf.Servers[i]
		for _, f := range r.renderServerFilters(s, peers) {
			conf.Filters = append(conf.Filters, f)
			s.Filters = append(s.Filters, f.Name)
		}
	}

	// validate so that defaults get filled in
	if err := conf.Validate(); err != nil {
		log.Error(err, "Invalid dataplane config", "config", conf.String())
		return NewCriticalError(RenderingError)
	}

	r.log.Info("Update queue ready", "queue", c.update.String())

	// config name is the name of the gateway
	gw := c.gws.GetFirst()
	if gw != nil {
		conf.Admin.Name = store.GetObjectKey(gw)

		// update cds server
		c.update.ConfigQueue = append(c.update.ConfigQueue, &conf)

		// create deployment
		dp, err := r.generateDataplane(c)
		if err != nil {
			return err
		}
		if isManagedDataplaneDisabled(gw) {
			if dep, ok := dp.(*appv1.Deployment); ok {
				c.update.DeleteQueue.Deployments.Upsert(dep.DeepCopy())
			} else if ds, ok := dp.(*appv1.DaemonSet); ok {
				c.update.DeleteQueue.DaemonSets.Upsert(ds.DeepCopy())
			}
			log.V(1).Info("Removing STUNner dataplane resource for Gateway",
				"gateway", store.DumpObject(gw),
				"disable-dataplane-annotation", true)
		} else {
			if dep, ok := dp.(*appv1.Deployment); ok {
				c.update.UpsertQueue.Deployments.Upsert(dep.DeepCopy())
				// delete lingering DeamonSets
				ds := &appv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
					Namespace: dep.GetNamespace(),
					Name:      dep.GetName(),
				}}
				c.update.DeleteQueue.DaemonSets.Upsert(ds)
			} else if ds, ok := dp.(*appv1.DaemonSet); ok {
				c.update.UpsertQueue.DaemonSets.Upsert(ds.DeepCopy())
				// delete lingering Deployments
				dep := &appv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Namespace: ds.GetNamespace(),
					Name:      ds.GetName(),
				}}
				c.update.DeleteQueue.Deployments.Upsert(dep)
			}
			log.Info("STUNner dataplane resource rendering ready",
				"generation", r.gen, "resource", store.DumpObject(dp))
		}
	}

	log.Info("STUNner dataplane configuration ready", "generation", r.gen, "config",
		conf.String())

	return nil
}

// invalidateGatewayClass invalidates an entire gateway-class, with all the gateways underneath
func (r *renderer) invalidateGatewayClass(c *RenderContext, reason error) {
	log := r.log
	gc := c.gc

	log.Info("Invalidating configuration", "gateway-class", store.GetObjectKey(gc),
		"reason", reason.Error())

	setGatewayClassStatusAccepted(gc, reason)
	c.update.UpsertQueue.GatewayClasses.Upsert(gc.DeepCopy())

	log.V(1).Info("Invalidating all gateway objects in gateway-class",
		"gateway-class", gc.GetName(), "reason", reason.Error())

	c.gws.ResetGateways(r.getGateways4Class(c))
	r.invalidateGateways(c, reason)
}

// invalidateGateways invalidates a set of Gateways
func (r *renderer) invalidateGateways(c *RenderContext, reason error) {
	log := r.log
	gc := c.gc

	for _, gw := range c.gws.GetAll() {
		log.V(2).Info("Considering", "gateway", store.GetObjectKey(gw),
			"listener-num", len(gw.Spec.Listeners))

		// this also re-inits listener statuses
		initGatewayStatus(gw, reason)

		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]
			setListenerStatus(gw, &l, errors.New("Invalid"), false, 0)
		}

		setGatewayStatusProgrammed(gw, reason, nil)
		gw = pruneGatewayStatusConds(gw)

		// schedule for update
		c.update.UpsertQueue.Gateways.Upsert(gw.DeepCopy())

		// delete the dataplane Service and the Deployment or DaemonSet of invalidated gateways;
		// we do not update the client via CDS: the dataplane is going away anyway
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gw.GetName(),
				Namespace: gw.GetNamespace(),
			},
		}
		c.update.DeleteQueue.Services.Upsert(svc)
		log.V(2).Info("Deleting dataplane Service", "generation", r.gen,
			"service", store.DumpObject(svc))

		dp := &appv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gw.GetName(),
				Namespace: gw.GetNamespace(),
			},
		}
		c.update.DeleteQueue.Deployments.Upsert(dp)
		log.V(2).Info("Deleting dataplane Deployment", "generation", r.gen,
			"deployment", store.DumpObject(dp))

		ds := &appv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      gw.GetName(),
				Namespace: gw.GetNamespace(),
			},
		}
		c.update.DeleteQueue.DaemonSets.Upsert(ds)
		log.V(2).Info("Deleting dataplane DaemonSet", "generation", r.gen,
			"daemonset", store.DumpObject(ds))
	}

	log.V(1).Info("Processing routes")
	for _, ro := range r.allRoutes() {
		log.V(2).Info("Considering", "route", ro.GetName())

		initRouteStatus(ro)

		parents := ro.GetParentRefs()
		for i := range parents {
			p := parents[i]

			// skip if we are not responsible
			if r.isParentOutContext(c.gws, ro, &p) {
				continue
			}

			parentExists, parentAccepted := r.isParentAcceptingRoute(ro, &p, gc.GetName())

			// render a stale cluster so that we know the ResolvedRefs status
			_, _, err := r.renderCluster(c, ro)

			setRouteConditionStatus(ro, &p, config.ControllerName, parentExists, parentAccepted, err)
		}

		queueRouteStatusUpdate(c, ro)
	}
}
