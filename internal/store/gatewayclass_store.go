package store

import (
	"k8s.io/apimachinery/pkg/types"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var GatewayClasses = NewGatewayClassStore()

type GatewayClassStore struct {
	Store
}

func NewGatewayClassStore() *GatewayClassStore {
	return &GatewayClassStore{
		Store: NewStore(),
	}
}

// GetAll returns all GatewayClass objects from the global storage
func (s *GatewayClassStore) GetAll() []*gatewayv1alpha2.GatewayClass {
	ret := make([]*gatewayv1alpha2.GatewayClass, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*gatewayv1alpha2.GatewayClass)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global GatewayClassStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named GatewayClass object from the global storage
func (s *GatewayClassStore) GetObject(nsName types.NamespacedName) *gatewayv1alpha2.GatewayClass {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*gatewayv1alpha2.GatewayClass)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global GatewayClassStore")
	}

	return r
}

// // AddGatewayClass adds a GatewayClass object to the the global storage (this is used mainly for testing)
// func (s *GatewayClassStore) AddGatewayClass(gc *gatewayv1alpha2.GatewayClass) {
// 	s.Upsert(gc)
// }
