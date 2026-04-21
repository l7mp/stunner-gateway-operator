package lens

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type GatewayClassLens struct {
	gwapiv1.GatewayClass `json:",inline"`
}

func NewGatewayClassLens(gc *gwapiv1.GatewayClass) *GatewayClassLens {
	return &GatewayClassLens{GatewayClass: *gc.DeepCopy()}
}

func (l *GatewayClassLens) EqualResource(_ client.Object) bool {
	return true
}

func (l *GatewayClassLens) ApplyToResource(_ client.Object) error {
	return nil
}

func (l *GatewayClassLens) EqualStatus(current client.Object) bool {
	gc, ok := current.(*gwapiv1.GatewayClass)
	if !ok {
		return false
	}

	return GatewayClassStatusEqual(gc.Status, &l.Status)
}

func (l *GatewayClassLens) ApplyToStatus(target client.Object) error {
	gc, ok := target.(*gwapiv1.GatewayClass)
	if !ok {
		return fmt.Errorf("gatewayclass lens: invalid target type %T", target)
	}

	l.Status.DeepCopyInto(&gc.Status)
	return nil
}

func (l *GatewayClassLens) DeepCopy() *GatewayClassLens {
	return &GatewayClassLens{GatewayClass: *l.GatewayClass.DeepCopy()}
}

func (l *GatewayClassLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

func GatewayClassStatusEqual(current gwapiv1.GatewayClassStatus, desired *gwapiv1.GatewayClassStatus) bool {
	normalized := desired.DeepCopy()
	normalizeConditionTimestamps(normalized.Conditions, current.Conditions)

	return apiequality.Semantic.DeepEqual(current, *normalized)
}
