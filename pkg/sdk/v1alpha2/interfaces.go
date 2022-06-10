package v1alpha2

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrgwv1alpha1 "github.com/l7mp/stunner-kubernetes-gateway/pkg/apis/gateway/v1alpha2"
)

type GatewayClassImpl interface {
	Upsert(gc *v1alpha2.GatewayClass)
	Remove(key string)
}

type GatewayImpl interface {
	Upsert(*v1alpha2.Gateway)
	Remove(string)
}

type GatewayConfigImpl interface {
	Upsert(config *stnrgwv1alpha1.GatewayConfig)
	Remove(string)
}

type HTTPRouteImpl interface {
	Upsert(config *v1alpha2.HTTPRoute)
	// FIXME(pleshakov): change other interfaces to use types.NamespacedName
	Remove(types.NamespacedName)
}

type ServiceImpl interface {
	Upsert(svc *apiv1.Service)
	Remove(name types.NamespacedName)
}