package store

import (
	"k8s.io/apimachinery/pkg/types"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var Gateways = NewGatewayStore()

type GatewayStore struct {
	Store
}

func NewGatewayStore() *GatewayStore {
	return &GatewayStore{
		Store: NewStore(),
	}
}

// GetAll returns all Gateway objects from the global storage
func (s *GatewayStore) GetAll() []*gatewayv1alpha2.Gateway {
	ret := make([]*gatewayv1alpha2.Gateway, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*gatewayv1alpha2.Gateway)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global GatewayStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Gateway object from the global storage
func (s *GatewayStore) GetObject(nsName types.NamespacedName) *gatewayv1alpha2.Gateway {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*gatewayv1alpha2.Gateway)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global GatewayStore")
	}

	return r
}

// // AddGateway adds a Gateway object to the the global storage (this is used mainly for testing)
// func (s *GatewayStore) AddGateway(gc *gatewayv1alpha2.Gateway) {
// 	s.Upsert(gc)
// }
