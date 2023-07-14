package store

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// GetFirst returns the first Gateway object from the storage
func (s *GatewayStore) GetFirst() *gatewayv1alpha2.Gateway {
	objects := s.Objects()
	if len(objects) == 0 {
		return nil
	}

	r, ok := objects[0].(*gatewayv1alpha2.Gateway)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in GatewayStore")
	}

	return r
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

// ResetGateways resets a Gateway store from a list of Gateways.
func (s *GatewayStore) ResetGateways(gws []*gatewayv1alpha2.Gateway) {
	// we have to make this conversion because Go won't do this for us automatically:
	// https://stackoverflow.com/questions/12994679/slice-of-struct-slice-of-interface-it-implements
	objs := make([]client.Object, len(gws))
	for i := range gws {
		objs[i] = gws[i]
	}
	s.Reset(objs)
}
