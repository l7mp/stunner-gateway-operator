package store

import (
	"k8s.io/apimachinery/pkg/types"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
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
func (s *GatewayClassStore) GetAll() []*gwapiv1.GatewayClass {
	ret := make([]*gwapiv1.GatewayClass, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*gwapiv1.GatewayClass)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global GatewayClassStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named GatewayClass object from the global storage
func (s *GatewayClassStore) GetObject(nsName types.NamespacedName) *gwapiv1.GatewayClass {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*gwapiv1.GatewayClass)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global GatewayClassStore")
	}

	return r
}

func (s *GatewayClassStore) DeepCopy() *GatewayClassStore {
	ret := NewGatewayClassStore()
	for _, o := range s.GetAll() {
		ret.Upsert(o)
	}
	return ret
}
