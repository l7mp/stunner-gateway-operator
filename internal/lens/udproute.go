package lens

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

type UDPRouteLens struct {
	stnrgwv1.UDPRoute `json:",inline"`
}

func NewUDPRouteLens(ro *stnrgwv1.UDPRoute) *UDPRouteLens {
	return &UDPRouteLens{UDPRoute: *ro.DeepCopy()}
}

func (l *UDPRouteLens) EqualResource(_ client.Object) bool {
	return true
}

func (l *UDPRouteLens) ApplyToResource(_ client.Object) error {
	return nil
}

func (l *UDPRouteLens) EqualStatus(current client.Object) bool {
	ro, ok := current.(*stnrgwv1.UDPRoute)
	if !ok {
		return false
	}

	return UDPRouteStatusEqual(ro.Status, l.Status)
}

func (l *UDPRouteLens) ApplyToStatus(target client.Object) error {
	ro, ok := target.(*stnrgwv1.UDPRoute)
	if !ok {
		return fmt.Errorf("udproute lens: invalid target type %T", target)
	}

	l.Status.DeepCopyInto(&ro.Status)
	return nil
}

func (l *UDPRouteLens) DeepCopy() *UDPRouteLens {
	return &UDPRouteLens{UDPRoute: *l.UDPRoute.DeepCopy()}
}

func (l *UDPRouteLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

type UDPRouteV1A2Lens struct {
	gwapiv1a2.UDPRoute `json:",inline"`
}

func NewUDPRouteV1A2Lens(ro *gwapiv1a2.UDPRoute) *UDPRouteV1A2Lens {
	return &UDPRouteV1A2Lens{UDPRoute: *ro.DeepCopy()}
}

func (l *UDPRouteV1A2Lens) EqualResource(_ client.Object) bool {
	return true
}

func (l *UDPRouteV1A2Lens) ApplyToResource(_ client.Object) error {
	return nil
}

func (l *UDPRouteV1A2Lens) EqualStatus(current client.Object) bool {
	ro, ok := current.(*gwapiv1a2.UDPRoute)
	if !ok {
		return false
	}

	return UDPRouteStatusEqual(ro.Status, l.Status)
}

func (l *UDPRouteV1A2Lens) ApplyToStatus(target client.Object) error {
	ro, ok := target.(*gwapiv1a2.UDPRoute)
	if !ok {
		return fmt.Errorf("udproute-v1a2 lens: invalid target type %T", target)
	}

	l.Status.DeepCopyInto(&ro.Status)
	return nil
}

func (l *UDPRouteV1A2Lens) DeepCopy() *UDPRouteV1A2Lens {
	return &UDPRouteV1A2Lens{UDPRoute: *l.UDPRoute.DeepCopy()}
}

func (l *UDPRouteV1A2Lens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

func UDPRouteStatusEqual(current, desired gwapiv1a2.UDPRouteStatus) bool {
	normalized := desired.DeepCopy()
	for i := range normalized.Parents {
		dp := &normalized.Parents[i]
		if cp := findRouteParentStatus(current.Parents, dp.ParentRef, dp.ControllerName); cp != nil {
			dp.ParentRef = cp.ParentRef
			normalizeConditionTimestamps(dp.Conditions, cp.Conditions)
		}
	}

	return apiequality.Semantic.DeepEqual(current, *normalized)
}

func findRouteParentStatus(ps []gwapiv1.RouteParentStatus, ref gwapiv1.ParentReference,
	controller gwapiv1.GatewayController) *gwapiv1.RouteParentStatus {
	for i := range ps {
		if ps[i].ControllerName != controller {
			continue
		}

		if parentRefEqual(ps[i].ParentRef, ref) {
			return &ps[i]
		}
	}

	return nil
}

func parentRefEqual(a, b gwapiv1.ParentReference) bool {
	return parentRefGroup(a.Group) == parentRefGroup(b.Group) &&
		parentRefKind(a.Kind) == parentRefKind(b.Kind) &&
		derefEq(a.Namespace, b.Namespace) &&
		a.Name == b.Name &&
		derefEq(a.SectionName, b.SectionName) &&
		derefEq(a.Port, b.Port)
}

func parentRefGroup(g *gwapiv1.Group) string {
	if g == nil || *g == "" {
		return gwapiv1.GroupName
	}

	return string(*g)
}

func parentRefKind(k *gwapiv1.Kind) string {
	if k == nil || *k == "" {
		return "Gateway"
	}

	return string(*k)
}

func derefEq[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	return *a == *b
}
