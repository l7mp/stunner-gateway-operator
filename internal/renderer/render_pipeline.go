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
	r.gen = e.Generation
	r.log.Info("Rendering configuration", "generation", r.gen, "event", e.String())

	switch config.DataplaneMode {
	case config.DataplaneModeLegacy:
		r.renderGatewayClass(e)
	case config.DataplaneModeManaged:
		r.renderManagedGateways(e)
	default:
		r.log.Info(`Unknown dataplane mode (must be either "managed" or "legacy")`)
		return
	}
}

// Finalize performs the finalization sequence:
// - set all managed Kubernetes statuses to invalid
// - remove managed dataplanes and services
func (r *Renderer) Finalize(e *event.EventFinalize) {
	r.gen = e.Generation

	switch config.DataplaneMode {
	case config.DataplaneModeLegacy:
		r.log.Info("Finalization not suported for legacy dataplane mode")
	case config.DataplaneModeManaged:
		r.finalizeManagedGateways(e)
	default:
		r.log.Info(`Finalizer: unknown dataplane mode (must be either "managed" or "legacy")`)
		return
	}
}

// renderGatewayClass generates and sets a STUNner daemon configuration in the "legacy" dataplane mode.
func (r *Renderer) renderGatewayClass(e *event.EventRender) {
	r.log.Info("Starting dataplane render", "mode", "legacy")

	r.log.V(1).Info("Obtaining gateway-class objects")
	gcs := r.getGatewayClasses()

	if len(gcs) == 0 {
		r.log.Info("No gateway-class objects found", "event", e.String())
		return
	}

	if len(gcs) > 1 {
		names := []string{}
		for _, gc := range gcs {
			names = append(names, fmt.Sprintf("%q", store.GetObjectKey(gc)))
		}

		r.log.Info("Multiple gateway-class objects found: this is most probably UNINTENED - "+
			"the operator will attempt to render a configuration for each gateway-class but there "+
			"is no guarantee that this will work - this mode is UNSUPPORTED, "+
			"if unsure, remove one of the gateway-class objects", "names", strings.Join(names, ", "))
	}

	// render each GatewayClass: hopefully they won's step on each other's throat: we cannot
	// help if multiple GatewayClasses (or the GatewayConfigs thereof) set the rendering
	// pipeline to render into the same configmap, but at least we can prevent race conditions
	// by serializing update requests on the updaterChannel
	for _, gc := range gcs {
		r.log.Info("Rendering configuration", "gateway-class", store.GetObjectKey(gc))
		c := NewRenderContext(r, gc)

		r.log.V(1).Info("Obtaining gateway-config", "gateway-class", gc.GetName())
		var err error
		c.gwConf, err = r.getGatewayConfig4Class(c)
		if err != nil {
			r.log.Error(err, "Error obtaining gateway-config",
				"gateway-class", gc.GetName())
			r.invalidateGatewayClass(c, err)
			continue
		}

		r.log.V(1).Info("Finding gateways", "gateway-class", store.GetObjectKey(gc))
		gws := r.getGateways4Class(c)
		c.gws.ResetGateways(gws)

		// render for ALL gateways that correspond to this gateway-class
		if err := r.renderForGateways(c); err != nil {
			// an irreparable error happened, invalidate the config and set all related
			// object statuses to signal the error
			r.log.Error(err, "Rendering error", "gateway-class", store.GetObjectKey(gc))
			r.invalidateGatewayClass(c, err)
		}

		setGatewayClassStatusAccepted(gc, nil)

		// send the update back to the operator
		r.operatorCh <- c.update.DeepCopy()
	}
}

// renderManagedGateways generates and sets a STUNner daemon configuration for the "managed" dataplane mode.
func (r *Renderer) renderManagedGateways(e *event.EventRender) {
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
			gw := gw

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

		pipelineCtx.Merge(gcCtx)
	}

	// updates must be acknowledged to the operator by the updater
	u := pipelineCtx.update.DeepCopy()
	u.SetRequestAck(true)
	r.operatorCh <- u
}

// finalizeManagedGateways invalidates all managed resources
func (r *Renderer) finalizeManagedGateways(e *event.EventFinalize) {
	r.log.Info("Stating finalization", "mode", "managed")

	pipelineCtx := NewRenderContext(r, nil)

	r.log.V(1).Info("Obtaining gateway-class objects")
	gcs := r.getGatewayClasses()
	if len(gcs) == 0 {
		r.log.Info("No gateway-class objects found", "event", e.String())
		return
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

	// finalization updates must be acknowledged to the operator by the updater
	u := pipelineCtx.update.DeepCopy()
	u.SetRequestAck(true)
	r.operatorCh <- u
}

// renderForGateways renders a configuration for a set of Gateways (c.gws)
func (r *Renderer) renderForGateways(c *RenderContext) error {
	log := r.log
	gc := c.gc

	conf := stnrconfv1.StunnerConfig{
		ApiVersion: stnrconfv1.ApiVersion,
	}

	targetName, targetNamespace := getTarget(c)

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
	conf.Auth = *auth

	conf.Listeners = []stnrconfv1.ListenerConfig{}
	for _, gw := range c.gws.GetAll() {
		log.V(2).Info("Considering", "gateway", store.GetObjectKey(gw), "listener-num",
			len(gw.Spec.Listeners))

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

			c.update.UpsertQueue.Services.Upsert(s)
			targetPorts = ports
		}

		udpPorts := make(map[int]bool)
		tcpPorts := make(map[int]bool)
		for j := range gw.Spec.Listeners {
			l := gw.Spec.Listeners[j]

			log.V(3).Info("Obtaining routes", "gateway", store.GetObjectKey(gw), "listener",
				l.Name)
			rs := r.getUDPRoutes4Listener(gw, &l)

			if isListenerConflicted(&l, udpPorts, tcpPorts) {
				log.Info("Listener protocol/port conflict", "gateway", store.GetObjectKey(gw),
					"listener", l.Name)
				setListenerStatus(gw, &l, NewNonCriticalError(PortUnavailable), true, len(rs))
				continue
			}

			// the Gateway may remap the listener's target port from an annotation:
			// this is indicated in targetPorts
			lc, err := r.renderListener(gw, c.gwConf, &l, rs, pubGwAddrs[j], targetPorts)
			if err != nil {
				// all listener rendering errors are critical: prevent the
				// rendering of the listener config
				log.Info("Error rendering configuration for listener", "gateway",
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

	log.V(1).Info("Processing UDPRoutes")
	conf.Clusters = []stnrconfv1.ClusterConfig{}
	for _, ro := range r.allUDPRoutes() {
		log.V(2).Info("Considering", "route", ro.GetName())

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
				log.Info("Non-critical error rendering cluster", "route",
					store.GetObjectKey(ro), "error", err.Error())
				// note error but otherwise ignore
				criticalErr = nil
			} else {
				log.Error(err, "Fatal error rendering cluster", "route",
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
	r.log.Info("Update queue ready", "queue", c.update.String())

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
			if isManagedDataplaneDisabled(gw) {
				c.update.DeleteQueue.Deployments.Upsert(dp)
				log.V(1).Info("Removing STUNner dataplane for Gateway",
					"gateway", store.DumpObject(gw),
					"disable-dataplane-annotation", true)
			} else {
				c.update.UpsertQueue.Deployments.Upsert(dp)
				log.Info("STUNner dataplane Deployment ready",
					"generation", r.gen, "deployment", store.DumpObject(dp))
			}
		}
	} else {
		cm, err := r.renderConfig(c, targetName, targetNamespace, &conf)
		if err != nil {
			return err
		}
		c.update.UpsertQueue.ConfigMaps.Upsert(cm)
	}

	log.Info("STUNner dataplane configuration ready", "generation", r.gen, "config",
		conf.String())

	return nil
}

// invalidateGatewayClass invalidates an entire gateway-class, with all the gateways underneath
func (r *Renderer) invalidateGatewayClass(c *RenderContext, reason error) {
	log := r.log
	gc := c.gc

	log.Info("Invalidating configuration", "gateway-class", store.GetObjectKey(gc),
		"reason", reason.Error())

	if config.DataplaneMode == config.DataplaneModeLegacy {
		if c.gwConf != nil {
			// remove the configmap
			targetNamespace := c.gwConf.GetNamespace()
			targetName := opdefault.DefaultConfigMapName
			if cm, err := r.renderConfig(c, targetName, targetNamespace, nil); err != nil {
				log.Error(err, "Error invalidating ConfigMap", "target",
					fmt.Sprintf("%s/%s", targetNamespace, targetName))
			} else {
				c.update.UpsertQueue.ConfigMaps.Upsert(cm)
			}
		} else {
			// this is the killer case: we have most probably lost our gatewayconfig
			// and we don't know which stunner config to invalidate; for now warn,
			// later eliminate such cases by putting a finalizer/owner-ref to
			// GatewayConfigs once we have started using them
			log.Info("No gateway-config: active STUNNer configuration may remain stale",
				"gateway-class", gc.GetName())
		}
	}

	setGatewayClassStatusAccepted(gc, reason)
	c.update.UpsertQueue.GatewayClasses.Upsert(gc)

	log.V(1).Info("Invalidating all gateway objects in gateway-class",
		"gateway-class", gc.GetName(), "reason", reason.Error())

	c.gws.ResetGateways(r.getGateways4Class(c))
	r.invalidateGateways(c, reason)
}

// invalidateGateways invalidates a set of Gateways
func (r *Renderer) invalidateGateways(c *RenderContext, reason error) {
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
		c.update.UpsertQueue.Gateways.Upsert(gw)

		// delete dataplane configmaps, services and deployments for invalidated gateways
		// we do not update the client via CDS: the deployment is going away anyway
		if config.DataplaneMode == config.DataplaneModeManaged {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gw.GetName(),
					Namespace: gw.GetNamespace(),
				},
			}
			c.update.DeleteQueue.ConfigMaps.Upsert(cm)
			log.V(2).Info("Deleting dataplane ConfigMap", "generation", r.gen,
				"deployment", store.DumpObject(cm))

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
		}
	}

	log.V(1).Info("Processing UDPRoutes")
	for _, ro := range r.allUDPRoutes() {
		log.V(2).Info("Considering", "route", ro.GetName())

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
