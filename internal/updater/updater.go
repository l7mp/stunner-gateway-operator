package updater

// updater uploads client updates
import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/metrics"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

type UpdaterConfig struct {
	Manager manager.Manager
	Logger  logr.Logger
}

// Updater applies the rendered Kubernetes resources and statuses to the cluster. It runs as a
// leader-only manager runnable: Start blocks until the context ends.
type Updater struct {
	manager   manager.Manager
	updaterCh chan event.Event
	opCh      chan<- event.Event
	statsMu   sync.Mutex
	stats     map[string]int64
	*config.ProgressTracker
	log logr.Logger
}

func NewUpdater(cfg UpdaterConfig) *Updater {
	return &Updater{
		manager:         cfg.Manager,
		updaterCh:       make(chan event.Event, 10),
		stats:           map[string]int64{},
		ProgressTracker: config.NewProgressTracker(),
		log:             cfg.Logger.WithName("updater"),
	}
}

func (u *Updater) incCounter(key string) {
	u.statsMu.Lock()
	u.stats[key]++
	u.statsMu.Unlock()
	// key format is "scope.Kind.operation"; update the Prometheus counter in parallel.
	if parts := strings.SplitN(key, ".", 3); len(parts) == 3 {
		metrics.ResourceOperationsTotal.WithLabelValues(parts[0], parts[1], parts[2]).Inc()
	}
}

func (u *Updater) SnapshotCounters() map[string]int64 {
	u.statsMu.Lock()
	defer u.statsMu.Unlock()

	out := make(map[string]int64, len(u.stats))
	for k, v := range u.stats {
		out[k] = v
	}

	return out
}

func (u *Updater) ResetCounters() {
	u.statsMu.Lock()
	u.stats = map[string]int64{}
	u.statsMu.Unlock()
}

// Start runs the update loop until the context ends.
func (u *Updater) Start(ctx context.Context) error {
	heartbeat := time.NewTicker(metrics.LoopHeartbeatInterval)
	defer heartbeat.Stop()
	metrics.RecordUpdaterHeartbeat()

	for {
		select {
		case <-heartbeat.C:
			metrics.RecordUpdaterHeartbeat()

		case e := <-u.updaterCh:
			metrics.RecordUpdaterHeartbeat()
			if e.GetType() != event.EventTypeUpdate {
				u.log.Info("Updater thread received unknown event", "event", e.String())
				continue
			}

			update := e.(*event.EventUpdate)

			u.ProgressUpdate(1)
			start := time.Now()
			err := u.ProcessUpdate(ctx, update)
			metrics.UpdateDuration.Observe(time.Since(start).Seconds())
			if err != nil {
				metrics.UpdateTotal.WithLabelValues("error").Inc()
				metrics.UpdateErrors.Inc()
				u.log.Error(err, "Could not process update event", "event", e.String())
			} else {
				metrics.UpdateTotal.WithLabelValues("success").Inc()
			}

			if update.GetRequestAck() {
				event.Send(ctx, u.opCh, event.NewEventAck(update.Generation))
			}

			u.ProgressUpdate(-1)

		case <-ctx.Done():
			return nil
		}
	}
}

// GetUpdaterChannel returns the channel on which the updater listenens to update resuests
func (u *Updater) GetUpdaterChannel() chan event.Event {
	return u.updaterCh
}

// ProcessUpdate applies one update event to the cluster.
func (u *Updater) ProcessUpdate(ctx context.Context, e *event.EventUpdate) error {
	gen := e.Generation
	u.log.Info("Processing update event", "generation", gen, "update", e.String())

	// run the upsert queue
	q := e.UpsertQueue

	for _, o := range q.GatewayClasses.Objects() {
		if err := u.updateStatusObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot update GatewayClass status", "gateway-class", store.DumpObject(o))
		}
	}

	for _, o := range q.Gateways.Objects() {
		if err := u.updateStatusObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot update Gateway status", "gateway", store.DumpObject(o))
		}
	}

	for _, o := range q.UDPRoutes.Objects() {
		if err := u.updateStatusObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot update UDPRoute status", "route", store.DumpObject(o))
		}
	}

	for _, o := range q.UDPRoutesGwAPI.Objects() {
		if err := u.updateStatusObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot update UDPRouteV1A2 status", "route", store.DumpObject(o))
		}
	}

	for _, o := range q.TCPRoutes.Objects() {
		if err := u.updateStatusObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot update TCPRoute status", "route", store.DumpObject(o))
		}
	}

	for _, o := range q.TCPRoutesGwAPI.Objects() {
		if err := u.updateStatusObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot update TCPRouteV1 status", "route", store.DumpObject(o))
		}
	}

	for _, o := range q.Services.Objects() {
		if op, err := u.upsertResourceObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot update Service", "operation", op,
				"service", store.DumpObject(o))
			continue
		}
	}

	for _, o := range q.ConfigMaps.Objects() {
		if op, err := u.upsertResourceObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot upsert ConfigMap", "operation", op,
				"config-map", store.DumpObject(o))
			continue
		}
	}

	for _, o := range q.Deployments.Objects() {
		if op, err := u.upsertResourceObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot upsert Deployment", "operation", op,
				"deployment", store.DumpObject(o))
			continue
		}
	}

	for _, o := range q.DaemonSets.Objects() {
		if op, err := u.upsertResourceObject(ctx, o, gen); err != nil {
			u.log.Error(err, "Cannot upsert DaemonSet", "operation", op,
				"daemonSet", store.DumpObject(o))
			continue
		}
	}

	// run the delete queue
	q = e.DeleteQueue
	for _, gc := range q.GatewayClasses.Objects() {
		if err := u.deleteObject(ctx, gc, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete GatewayClass", "gateway-class",
				store.DumpObject(gc), "error", err)
			continue
		}
	}

	for _, gw := range q.Gateways.Objects() {
		if err := u.deleteObject(ctx, gw, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete Gateway", "gateway",
				store.DumpObject(gw), "error", err)
			continue
		}
	}

	for _, ro := range q.UDPRoutes.Objects() {
		if err := u.deleteObject(ctx, ro, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete UDPRoute", "route",
				store.DumpObject(ro), "error", err)
			continue
		}
	}

	for _, ro := range q.UDPRoutesGwAPI.Objects() {
		if err := u.deleteObject(ctx, ro, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete UDPRouteV1A2", "route",
				store.DumpObject(ro), "error", err)
			continue
		}
	}

	for _, ro := range q.TCPRoutes.Objects() {
		if err := u.deleteObject(ctx, ro, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete TCPRoute", "route",
				store.DumpObject(ro), "error", err)
			continue
		}
	}

	for _, ro := range q.TCPRoutesGwAPI.Objects() {
		if err := u.deleteObject(ctx, ro, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete TCPRouteV1", "route",
				store.DumpObject(ro), "error", err)
			continue
		}
	}

	for _, svc := range q.Services.Objects() {
		if err := u.deleteObject(ctx, svc, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete Service", "service",
				store.DumpObject(svc), "error", err)
			continue
		}
	}

	for _, cm := range q.ConfigMaps.Objects() {
		if err := u.deleteObject(ctx, cm, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete config-map", "config-map",
				store.DumpObject(cm), "error", err)
			continue
		}
	}

	for _, dp := range q.Deployments.Objects() {
		if err := u.deleteObject(ctx, dp, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete deployment", "deployment",
				store.DumpObject(dp), "error", err)
			continue
		}
	}

	for _, ds := range q.DaemonSets.Objects() {
		if err := u.deleteObject(ctx, ds, gen); err != nil && !apierrors.IsNotFound(err) {
			u.log.V(1).Info("Cannot delete daemonSet", "daemonSet",
				store.DumpObject(ds), "error", err)
			continue
		}
	}

	return nil
}

// SetAckChannel sets the channel to send acks to the operator.
func (u *Updater) SetAckChannel(ch chan<- event.Event) {
	u.opCh = ch
}
