package store

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var Namespaces = NewNamespaceStore()

type NamespaceStore struct {
	Store
}

func NewNamespaceStore() *NamespaceStore {
	return &NamespaceStore{
		Store: NewStore(),
	}
}

// GetAll returns all Namespace objects from the global storage
func (s *NamespaceStore) GetAll() []*corev1.Namespace {
	ret := make([]*corev1.Namespace, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*corev1.Namespace)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global NamespaceStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Namespace object from the global storage
func (s *NamespaceStore) GetObject(nsName types.NamespacedName) *corev1.Namespace {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*corev1.Namespace)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global NamespaceStore")
	}

	return r
}

// // AddNamespace adds a Namespace object to the the global storage (this is used mainly for testing)
// func (s *NamespaceStore) AddNamespace(gc *gatewayv1alpha2.Namespace) {
// 	s.Upsert(gc)
// }
