package store

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
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
func (s *GatewayStore) GetAll() []*gwapiv1.Gateway {
	ret := make([]*gwapiv1.Gateway, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*gwapiv1.Gateway)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global GatewayStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetFirst returns the first Gateway object from the storage
func (s *GatewayStore) GetFirst() *gwapiv1.Gateway {
	objects := s.Objects()
	if len(objects) == 0 {
		return nil
	}

	r, ok := objects[0].(*gwapiv1.Gateway)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in GatewayStore")
	}

	return r
}

// GetObject returns a named Gateway object from the global storage
func (s *GatewayStore) GetObject(nsName types.NamespacedName) *gwapiv1.Gateway {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*gwapiv1.Gateway)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global GatewayStore")
	}

	return r
}

// // AddGateway adds a Gateway object to the the global storage (this is used mainly for testing)
// func (s *GatewayStore) AddGateway(gc *gwapiv1.Gateway) {
// 	s.Upsert(gc)
// }

// ResetGateways resets a Gateway store from a list of Gateways.
func (s *GatewayStore) ResetGateways(gws []*gwapiv1.Gateway) {
	// we have to make this conversion because Go won't do this for us automatically:
	// https://stackoverflow.com/questions/12994679/slice-of-struct-slice-of-interface-it-implements
	objs := make([]client.Object, len(gws))
	for i := range gws {
		objs[i] = gws[i]
	}
	s.Reset(objs)
}

func (s *GatewayStore) DeepCopy() *GatewayStore {
	ret := NewGatewayStore()
	for _, o := range s.GetAll() {
		ret.Upsert(o.DeepCopy())
	}
	return ret
}
