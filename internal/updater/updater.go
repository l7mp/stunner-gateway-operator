package updater

// updater uploads client updates
import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

type UpdaterConfig struct {
	Manager manager.Manager
	Logger  logr.Logger
}

type Updater struct {
	ctx       context.Context
	manager   manager.Manager
	gen       int
	updaterCh chan event.Event
	log       logr.Logger
}

func NewUpdater(cfg UpdaterConfig) *Updater {
	return &Updater{
		manager:   cfg.Manager,
		gen:       0,
		updaterCh: make(chan event.Event, 5),
		log:       cfg.Logger.WithName("updater"),
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

				// FIXME should run in separate thread
				err := u.ProcessUpdate(e.(*event.EventUpdate))

				if err != nil {
					u.log.Error(err, "could not update process event", "event",
						e.String())
				}

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
	u.gen += 1
	u.log.Info("processing update event", "generation", u.gen, "update", e.String())

	client := u.manager.GetClient()

	for _, gc := range e.GatewayClasses.Objects() {
		u.log.V(2).Info("updating gateway classes", "resource",
			store.GetObjectKey(gc))

		if err := client.Update(u.ctx, gc); err != nil {
			return fmt.Errorf("cannot update gatewayclass %q: %w",
				store.GetObjectKey(gc), err)
		}
	}

	for _, gw := range e.Gateways.Objects() {
		u.log.V(2).Info("updating gateways", "resource",
			store.GetObjectKey(gw))

		if err := client.Update(u.ctx, gw); err != nil {
			return fmt.Errorf("cannot update gateway %q: %w",
				store.GetObjectKey(gw), err)
		}
	}

	for _, ro := range e.UDPRoutes.Objects() {
		u.log.V(2).Info("updating gateway classes", "resource",
			store.GetObjectKey(ro))

		if err := client.Update(u.ctx, ro); err != nil {
			return fmt.Errorf("cannot update UDP route %q: %w",
				store.GetObjectKey(ro), err)
		}
	}

	cms := e.ConfigMaps.Objects()
	if len(cms) != 1 {
		return fmt.Errorf("invalid number (%d) of STUNner ConfigMaps to update, "+
			"should be 1", len(cms))
	}

	cm := cms[0]
	u.log.V(2).Info("updating STUNner ConfigMap", "resource",
		store.GetObjectKey(cm))

	cmCurrent := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name:      cm.GetName(),
		Namespace: cm.GetNamespace(),
	}}

	op, err := controllerutil.CreateOrUpdate(u.ctx, client, cmCurrent, func() error {
		cmCurrent = cm.(*corev1.ConfigMap)
		return nil
	})

	if err != nil {
		return fmt.Errorf("cannot create-or-update STUNner ConfigMap %q: %w",
			store.GetObjectKey(cm), err)
	}

	u.log.V(1).Info("all objects successfully updated", "generation", u.gen, "configmap",
		op)

	return nil
}
