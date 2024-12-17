package store

import (
	appv1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/types"
)

var DaemonSets = NewDaemonSetStore()

type DaemonSetStore struct {
	Store
}

func NewDaemonSetStore() *DaemonSetStore {
	return &DaemonSetStore{
		Store: NewStore(),
	}
}

// GetAll returns all DaemonSet objects from the global storage
func (s *DaemonSetStore) GetAll() []*appv1.DaemonSet {
	ret := make([]*appv1.DaemonSet, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*appv1.DaemonSet)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global DaemonSetStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named DaemonSet object from the global storage
func (s *DaemonSetStore) GetObject(nsName types.NamespacedName) *appv1.DaemonSet {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*appv1.DaemonSet)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global DaemonSetStore")
	}

	return r
}

func (s *DaemonSetStore) DeepCopy() *DaemonSetStore {
	ret := NewDaemonSetStore()
	for _, o := range s.GetAll() {
		ret.Upsert(o.DeepCopy())
	}
	return ret
}
