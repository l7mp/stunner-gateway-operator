package store

import (
	"k8s.io/apimachinery/pkg/types"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
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
func (s *DataplaneStore) GetAll() []*stnrgwv1.Dataplane {
	ret := make([]*stnrgwv1.Dataplane, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*stnrgwv1.Dataplane)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global DataplaneStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Dataplane object from the global storage
func (s *DataplaneStore) GetObject(nsName types.NamespacedName) *stnrgwv1.Dataplane {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*stnrgwv1.Dataplane)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global DataplaneStore")
	}

	return r
}

func (s *DataplaneStore) DeepCopy() *DataplaneStore {
	ret := NewDataplaneStore()
	for _, o := range s.GetAll() {
		ret.Upsert(o.DeepCopy())
	}
	return ret
}
