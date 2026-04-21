package lens

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type GatewayLens struct {
	gwapiv1.Gateway `json:",inline"`
}

func NewGatewayLens(gw *gwapiv1.Gateway) *GatewayLens {
	return &GatewayLens{Gateway: *gw.DeepCopy()}
}

func (l *GatewayLens) EqualResource(_ client.Object) bool {
	return true
}

func (l *GatewayLens) ApplyToResource(_ client.Object) error {
	return nil
}

func (l *GatewayLens) EqualStatus(current client.Object) bool {
	gw, ok := current.(*gwapiv1.Gateway)
	if !ok {
		return false
	}

	return GatewayStatusEqual(gw.Status, &l.Status)
}

func (l *GatewayLens) ApplyToStatus(target client.Object) error {
	gw, ok := target.(*gwapiv1.Gateway)
	if !ok {
		return fmt.Errorf("gateway lens: invalid target type %T", target)
	}

	l.Status.DeepCopyInto(&gw.Status)
	return nil
}

func (l *GatewayLens) DeepCopy() *GatewayLens {
	return &GatewayLens{Gateway: *l.Gateway.DeepCopy()}
}

func (l *GatewayLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

func GatewayStatusEqual(current gwapiv1.GatewayStatus, desired *gwapiv1.GatewayStatus) bool {
	normalized := desired.DeepCopy()
	normalizeConditionTimestamps(normalized.Conditions, current.Conditions)
	for i := range normalized.Listeners {
		dl := &normalized.Listeners[i]
		if cl := findListenerStatus(current.Listeners, dl.Name); cl != nil {
			normalizeConditionTimestamps(dl.Conditions, cl.Conditions)
		}
	}

	return apiequality.Semantic.DeepEqual(current, *normalized)
}

func normalizeConditionTimestamps(dst, src []metav1.Condition) {
	for i := range dst {
		d := &dst[i]
		s := meta.FindStatusCondition(src, d.Type)
		if s == nil {
			continue
		}

		if s.Status == d.Status &&
			s.Reason == d.Reason &&
			s.Message == d.Message &&
			s.ObservedGeneration == d.ObservedGeneration {
			d.LastTransitionTime = s.LastTransitionTime
		}
	}
}

func findListenerStatus(ls []gwapiv1.ListenerStatus, name gwapiv1.SectionName) *gwapiv1.ListenerStatus {
	for i := range ls {
		if ls[i].Name == name {
			return &ls[i]
		}
	}

	return nil
}
