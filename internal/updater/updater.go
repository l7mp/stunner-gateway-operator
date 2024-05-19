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

		for {
			select {
			case e := <-u.updaterCh:
				if e.GetType() != event.EventTypeUpdate {
					u.log.Info("updater thread received unknown event",
						"event", e.String())
					continue
				}

				u.ProgressUpdate(1)
				err := u.ProcessUpdate(e.(*event.EventUpdate))
				if err != nil {
					u.log.Error(err, "could not process update event", "event",
						e.String())
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
	u.log.Info("processing update event", "generation", gen, "update", e.String())

	// run the upsert queue
	q := e.UpsertQueue
	for _, gc := range q.GatewayClasses.GetAll() {
		if err := u.updateGatewayClass(gc, gen); err != nil {
			u.log.Error(err, "cannot update gateway-class",
				"gateway-class", store.DumpObject(gc))
			continue
		}
	}

	for _, gw := range q.Gateways.GetAll() {
		if err := u.updateGateway(gw, gen); err != nil {
			u.log.Error(err, "cannot update gateway",
				"gateway", store.DumpObject(gw))
			continue
		}
	}

	for _, ro := range q.UDPRoutes.GetAll() {
		if err := u.updateUDPRoute(ro, gen); err != nil {
			u.log.Error(err, "cannot update UDP route",
				"route", store.DumpObject(ro))
			continue
		}
	}

	for _, ro := range q.UDPRoutesV1A2.GetAll() {
		if err := u.updateUDPRouteV1A2(ro, gen); err != nil {
			u.log.Error(err, "cannot update UDPRouteV1A2", "route",
				store.DumpObject(stnrgwv1.ConvertV1UDPRouteToV1A2(ro)))
			continue
		}
	}

	for _, svc := range q.Services.GetAll() {
		if op, err := u.upsertService(svc, gen); err != nil {
			u.log.Error(err, "cannot update service", "operation", op,
				"service", store.DumpObject(svc))
			continue
		}
	}

	for _, cm := range q.ConfigMaps.GetAll() {
		if op, err := u.upsertConfigMap(cm, gen); err != nil {
			u.log.Error(err, "cannot upsert config-map", "operation", op,
				"config-map", store.DumpObject(cm))
			continue
		}
	}

	for _, dp := range q.Deployments.GetAll() {
		if op, err := u.upsertDeployment(dp, gen); err != nil {
			u.log.Error(err, "cannot upsert deployment", "operation", op,
				"deployment", store.DumpObject(dp))
			continue
		}
	}

	// run the delete queue
	q = e.DeleteQueue
	for _, gc := range q.GatewayClasses.Objects() {
		if err := u.deleteObject(gc, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("cannot delete gateway-class", "gateway-class",
				store.DumpObject(gc), "error", err)
			continue
		}
	}

	for _, gw := range q.Gateways.Objects() {
		if err := u.deleteObject(gw, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("cannot delete gateway", "gateway",
				store.DumpObject(gw), "error", err)
			continue
		}
	}

	for _, ro := range q.UDPRoutes.Objects() {
		if err := u.deleteObject(ro, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("cannot delete UDP route", "route",
				store.DumpObject(ro), "error", err)
			continue
		}
	}

	for _, svc := range q.Services.Objects() {
		if err := u.deleteObject(svc, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("cannot delete service", "service",
				store.DumpObject(svc), "error", err)
			continue
		}
	}

	for _, cm := range q.ConfigMaps.Objects() {
		if err := u.deleteObject(cm, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("cannot delete config-map", "config-map",
				store.DumpObject(cm), "error", err)
			continue
		}
	}

	for _, dp := range q.Deployments.Objects() {
		if err := u.deleteObject(dp, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("cannot delete deployment",
				"deployment", store.DumpObject(dp), "error", err)
			continue
		}
	}

	return nil
}
