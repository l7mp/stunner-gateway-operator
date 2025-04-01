package updater

// updater uploads client updates
import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

type UpdaterConfig struct {
	Manager manager.Manager
	Logger  logr.Logger
}

type Updater struct {
	ctx       context.Context
	manager   manager.Manager
	updaterCh chan event.Event
	opCh      event.EventChannel
	*config.ProgressTracker
	log logr.Logger
}

func NewUpdater(cfg UpdaterConfig) *Updater {
	return &Updater{
		manager:         cfg.Manager,
		updaterCh:       make(chan event.Event, 10),
		ProgressTracker: config.NewProgressTracker(),
		log:             cfg.Logger.WithName("updater"),
	}
}

func (u *Updater) Start(ctx context.Context) error {
	u.ctx = ctx

	go func() {
		defer close(u.updaterCh)
		defer u.opCh.Put()

		for {
			select {
			case e := <-u.updaterCh:
				if e.GetType() != event.EventTypeUpdate {
					u.log.Info("Updater thread received unknown event",
						"event", e.String())
					continue
				}

				update := e.(*event.EventUpdate)

				u.ProgressUpdate(1)
				err := u.ProcessUpdate(update)
				if err != nil {
					u.log.Error(err, "Could not process update event", "event",
						e.String())
				}

				if update.GetRequestAck() {
					u.opCh.Channel() <- event.NewEventAck(update.Generation)
				}

				u.ProgressUpdate(-1)

			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// GetUpdaterChannel returns the channel on which the updater listenens to update resuests
func (u *Updater) GetUpdaterChannel() chan event.Event {
	return u.updaterCh
}

func (u *Updater) ProcessUpdate(e *event.EventUpdate) error {
	gen := e.Generation
	u.log.Info("Processing update event", "generation", gen, "update", e.String())

	// run the upsert queue
	q := e.UpsertQueue
	for _, gc := range q.GatewayClasses.GetAll() {
		if err := u.updateGatewayClass(gc, gen); err != nil {
			u.log.Error(err, "Cannot update GatewayClass", "gateway-class",
				store.DumpObject(gc))
			continue
		}
	}

	for _, gw := range q.Gateways.GetAll() {
		if err := u.updateGateway(gw, gen); err != nil {
			u.log.Error(err, "Cannot update Gateway", "gateway", store.DumpObject(gw))
			continue
		}
	}

	for _, ro := range q.UDPRoutes.GetAll() {
		if err := u.updateUDPRoute(ro, gen); err != nil {
			u.log.Error(err, "Cannot update UDP route", "route", store.DumpObject(ro))
			continue
		}
	}

	for _, ro := range q.UDPRoutesV1A2.GetAll() {
		if err := u.updateUDPRouteV1A2(ro, gen); err != nil {
			u.log.Error(err, "Cannot update UDPRouteV1A2", "route",
				store.DumpObject(stnrgwv1.ConvertV1UDPRouteToV1A2(ro)))
			continue
		}
	}

	for _, svc := range q.Services.GetAll() {
		if op, err := u.upsertService(svc, gen); err != nil {
			u.log.Error(err, "Cannot update Service", "operation", op,
				"service", store.DumpObject(svc))
			continue
		}
	}

	for _, cm := range q.ConfigMaps.GetAll() {
		if op, err := u.upsertConfigMap(cm, gen); err != nil {
			u.log.Error(err, "Cannot upsert ConfigMap", "operation", op,
				"config-map", store.DumpObject(cm))
			continue
		}
	}

	for _, dp := range q.Deployments.GetAll() {
		if op, err := u.upsertDeployment(dp, gen); err != nil {
			u.log.Error(err, "Cannot upsert Deployment", "operation", op,
				"deployment", store.DumpObject(dp))
			continue
		}
	}

	for _, ds := range q.DaemonSets.GetAll() {
		if op, err := u.upsertDaemonSet(ds, gen); err != nil {
			u.log.Error(err, "Cannot upsert DaemonSet", "operation", op,
				"daemonSet", store.DumpObject(ds))
			continue
		}
	}

	// run the delete queue
	q = e.DeleteQueue
	for _, gc := range q.GatewayClasses.Objects() {
		if err := u.deleteObject(gc, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete GatewayClass", "gateway-class",
				store.DumpObject(gc), "error", err)
			continue
		}
	}

	for _, gw := range q.Gateways.Objects() {
		if err := u.deleteObject(gw, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete Gateway", "gateway",
				store.DumpObject(gw), "error", err)
			continue
		}
	}

	for _, ro := range q.UDPRoutes.Objects() {
		if err := u.deleteObject(ro, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete UDPRoute", "route",
				store.DumpObject(ro), "error", err)
			continue
		}
	}

	for _, svc := range q.Services.Objects() {
		if err := u.deleteObject(svc, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete Service", "service",
				store.DumpObject(svc), "error", err)
			continue
		}
	}

	for _, cm := range q.ConfigMaps.Objects() {
		if err := u.deleteObject(cm, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete config-map", "config-map",
				store.DumpObject(cm), "error", err)
			continue
		}
	}

	for _, dp := range q.Deployments.Objects() {
		if err := u.deleteObject(dp, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete deployment", "deployment",
				store.DumpObject(dp), "error", err)
			continue
		}
	}

	for _, ds := range q.DaemonSets.Objects() {
		if err := u.deleteObject(ds, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete daemonSet", "daemonSet",
				store.DumpObject(ds), "error", err)
			continue
		}
	}

	return nil
}

// SetAckChannel sets the channel to send acks to the operator.
func (u *Updater) SetAckChannel(ch event.EventChannel) {
	u.opCh = ch
	ch.Get()
}
