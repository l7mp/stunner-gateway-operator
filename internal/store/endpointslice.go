package store

import (
	discoveryv1 "k8s.io/api/discovery/v1"

	"k8s.io/apimachinery/pkg/types"
)

var EndpointSlices = NewEndpointSliceStore()

type EndpointSliceStore struct {
	Store
}

func NewEndpointSliceStore() *EndpointSliceStore {
	return &EndpointSliceStore{
		Store: NewStore(),
	}
}

// GetAll returns all EndpointSlice objects from the global storage
func (s *EndpointSliceStore) GetAll() []*discoveryv1.EndpointSlice {
	ret := make([]*discoveryv1.EndpointSlice, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*discoveryv1.EndpointSlice)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global EndpointSliceStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named EndpointSlice object from the global storage
func (s *EndpointSliceStore) GetObject(nsName types.NamespacedName) *discoveryv1.EndpointSlice {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*discoveryv1.EndpointSlice)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global EndpointSliceStore")
	}

	return r
}
