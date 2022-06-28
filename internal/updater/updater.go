package updater

// updater uploads client updates
import (
	"context"
	// "fmt"
	// "reflect"

	"github.com/go-logr/logr"

	// corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// corev1 "k8s.io/api/core/v1"
	// "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"
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

	for _, gc := range e.GatewayClasses.Objects() {
		if err := u.updateGatewayClass(gc); err != nil {
			u.log.Error(err, "cannot update gateway-class")
			continue
		}
	}

	for _, gw := range e.Gateways.Objects() {
		if err := u.updateGateway(gw); err != nil {
			u.log.Error(err, "cannot update gateway")
			continue
		}
	}

	for _, ro := range e.UDPRoutes.Objects() {
		if err := u.updateUDPRoute(ro); err != nil {
			u.log.Error(err, "cannot update UDP route")
			continue
		}
	}

	// there should never be more than one configmap but anyway
	for _, cm := range e.ConfigMaps.Objects() {
		if op, err := u.updateConfigMap(cm); err != nil {
			u.log.Error(err, "cannot update STUNner configuration", "operation", op)
			continue
		}
	}

	return nil
}
