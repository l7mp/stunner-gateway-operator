package lens

import (
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// RouteLens is the lens for every route kind (STUNner UDPRoute, TCPRoute and TURNRoute, Gateway API
// UDPRoute and TCPRoute): routes are all status-only from the operator's viewpoint and their status
// is exactly a gwapiv1.RouteStatus, so one lens serves all kinds.
type RouteLens struct {
	client.Object
}

func NewRouteLens(ro client.Object) *RouteLens {
	return &RouteLens{Object: ro.DeepCopyObject().(client.Object)}
}

// routeStatus returns the RouteStatus of any supported route kind, or nil for non-route objects.
func routeStatus(o client.Object) *gwapiv1.RouteStatus {
	switch ro := o.(type) {
	case *stnrgwv1.UDPRoute:
		return &ro.Status.RouteStatus
	case *gwapiv1.UDPRoute:
		return &ro.Status.RouteStatus
	case *gwapiv1a2.UDPRoute:
		return &ro.Status.RouteStatus
	case *stnrgwv1.TCPRoute:
		return &ro.Status.RouteStatus
	case *gwapiv1.TCPRoute:
		return &ro.Status.RouteStatus
	case *gwapiv1a2.TCPRoute:
		return &ro.Status.RouteStatus
	case *stnrgwv1a1.TURNRoute:
		return &ro.Status
	}
	return nil
}

func (l *RouteLens) EqualResource(_ client.Object) bool {
	return true
}

func (l *RouteLens) ApplyToResource(_ client.Object) error {
	return nil
}

func (l *RouteLens) EqualStatus(current client.Object) bool {
	cs := routeStatus(current)
	if cs == nil {
		return false
	}

	return RouteStatusEqual(*cs, *routeStatus(l.Object))
}

func (l *RouteLens) ApplyToStatus(target client.Object) error {
	ts := routeStatus(target)
	if ts == nil {
		return fmt.Errorf("route lens: invalid target type %T", target)
	}

	routeStatus(l.Object).DeepCopyInto(ts)
	return nil
}

func (l *RouteLens) DeepCopy() *RouteLens {
	return &RouteLens{Object: l.Object.DeepCopyObject().(client.Object)}
}

func (l *RouteLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

// RouteStatusEqual compares the status of two routes of any kind, ignoring differences in
// condition timestamps and the representation of default-valued parent reference fields.
func RouteStatusEqual(current, desired gwapiv1.RouteStatus) bool {
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
