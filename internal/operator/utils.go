package operator

import (
	// "fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// GetManager returns the controller manager associated with this operator
func (o *Operator) GetManager() manager.Manager {
	return o.manager
}

// GetLogger returns the logger associated with this operator
func (o *Operator) GetLogger() logr.Logger {
	return o.logger
}

// GetOperatorChannel returns the channel on which the operator event dispatcher listens
func (o *Operator) GetOperatorChannel() chan event.Event {
	return o.operatorCh
}

// // GetControllerName returns the controller-name (as per GatewayClass.Spec.ControllerName)
// // associated with this operator
// func (o *Operator) GetControllerName() string {
// 	return o.controllerName
// }
