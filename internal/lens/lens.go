package lens

import (
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

type Lens interface {
	client.Object
	EqualResource(current client.Object) bool
	ApplyToResource(target client.Object) error
	EqualStatus(current client.Object) bool
	ApplyToStatus(target client.Object) error
}

func New(o client.Object) (Lens, error) {
	switch current := o.(type) {
	case *corev1.ConfigMap:
		return NewConfigMapLens(current), nil
	case *corev1.Service:
		return NewServiceLens(current), nil
	case *appv1.Deployment:
		return NewDeploymentLens(current), nil
	case *appv1.DaemonSet:
		return NewDaemonSetLens(current), nil
	case *gwapiv1.GatewayClass:
		return NewGatewayClassLens(current), nil
	case *gwapiv1.Gateway:
		return NewGatewayLens(current), nil
	case *stnrgwv1.UDPRoute:
		return NewUDPRouteLens(current), nil
	case *gwapiv1a2.UDPRoute:
		return NewUDPRouteV1A2Lens(current), nil
	default:
		return nil, fmt.Errorf("unsupported object type %T", o)
	}
}
