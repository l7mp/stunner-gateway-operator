package store

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
)

var Services = NewServiceStore()

type ServiceStore struct {
	Store
}

func NewServiceStore() *ServiceStore {
	return &ServiceStore{
		Store: NewStore(),
	}
}

// GetAll returns all Service objects from the global storage
func (s *ServiceStore) GetAll() []*corev1.Service {
	ret := make([]*corev1.Service, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*corev1.Service)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global ServiceStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Service object from the global storage
func (s *ServiceStore) GetObject(nsName types.NamespacedName) *corev1.Service {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*corev1.Service)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global ServiceStore")
	}

	return r
}

func (s *ServiceStore) DeepCopy() *ServiceStore {
	ret := NewServiceStore()
	for _, o := range s.GetAll() {
		ret.Upsert(o)
	}
	return ret
}
