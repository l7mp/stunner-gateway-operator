package updater

// updater uploads client updates
import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"

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
	updaterCh chan event.Event
	log       logr.Logger
}

func NewUpdater(cfg UpdaterConfig) *Updater {
	return &Updater{
		updaterCh: make(chan event.Event, 5),
		manager:   cfg.Manager,
		log:       cfg.Logger,
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

func (u *Updater) ProcessUpdate(e *event.EventUpdate) error {
	u.log.V(1).Info("ProcessUpdate: processing update event", "update", e.String())
	client := u.manager.GetClient()

	u.log.V(2).Info("ProcessUpdate: updating gateway classes")
	for _, gc := range e.GatewayClasses.Objects() {
		if err := client.Update(u.ctx, gc); err != nil {
			return fmt.Errorf("cannot update gatewayclass %q: %w",
				store.GetObjectKey(gc), err)
		}
	}

	u.log.V(2).Info("ProcessUpdate: updating gateways")
	for _, gw := range e.Gateways.Objects() {
		if err := client.Update(u.ctx, gw); err != nil {
			return fmt.Errorf("cannot update gateway %q: %w",
				store.GetObjectKey(gw), err)
		}
	}

	u.log.V(2).Info("ProcessUpdate: updating UDP routes")
	for _, ro := range e.UDPRoutes.Objects() {
		if err := client.Update(u.ctx, ro); err != nil {
			return fmt.Errorf("cannot update UDP route %q: %w",
				store.GetObjectKey(ro), err)
		}
	}

	u.log.V(2).Info("ProcessUpdate: updating STUNner ConfigMap")
	cms := e.UDPRoutes.Objects()
	if len(cms) != 1 {
		return fmt.Errorf("invalid number of STUNner ConfigMaps to update: %s",
			"should be 1")
	}
	for _, cm := range cms {
		if err := client.Update(u.ctx, cm); err != nil {
			return fmt.Errorf("cannot update ConfigMap %q: %w",
				store.GetObjectKey(cm), err)
		}
	}

	return nil
}
