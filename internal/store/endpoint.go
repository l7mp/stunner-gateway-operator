//nolint:staticcheck
package store

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
)

var Endpoints = NewEndpointStore()

type EndpointStore struct {
	Store
}

func NewEndpointStore() *EndpointStore {
	return &EndpointStore{
		Store: NewStore(),
	}
}

// GetAll returns all Endpoint objects from the global storage
func (s *EndpointStore) GetAll() []*corev1.Endpoints {
	ret := make([]*corev1.Endpoints, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*corev1.Endpoints)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global EndpointStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Endpoint object from the global storage
func (s *EndpointStore) GetObject(nsName types.NamespacedName) *corev1.Endpoints {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*corev1.Endpoints)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global EndpointStore")
	}

	return r
}
