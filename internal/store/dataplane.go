package store

import (
	"k8s.io/apimachinery/pkg/types"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var Dataplanes = NewDataplaneStore()

type DataplaneStore struct {
	Store
}

func NewDataplaneStore() *DataplaneStore {
	return &DataplaneStore{
		Store: NewStore(),
	}
}

// GetAll returns all Dataplane objects from the global storage.
func (s *DataplaneStore) GetAll() []*stunnerv1alpha1.Dataplane {
	ret := make([]*stunnerv1alpha1.Dataplane, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*stunnerv1alpha1.Dataplane)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global DataplaneStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Dataplane object from the global storage
func (s *DataplaneStore) GetObject(nsName types.NamespacedName) *stunnerv1alpha1.Dataplane {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*stunnerv1alpha1.Dataplane)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global DataplaneStore")
	}

	return r
}

// // AddDataplane adds a Dataplane object to the the global storage (this is used mainly for testing)
// func (s *DataplaneStore) AddDataplane(gc *stunnerv1alpha1.Dataplane) {
// 	s.Upsert(gc)
// }
