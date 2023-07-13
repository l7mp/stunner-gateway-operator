package store

import (
	"k8s.io/apimachinery/pkg/types"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var StaticServices = NewStaticServiceStore()

type StaticServiceStore struct {
	Store
}

func NewStaticServiceStore() *StaticServiceStore {
	return &StaticServiceStore{
		Store: NewStore(),
	}
}

// GetAll returns all StaticService objects from the global storage
func (s *StaticServiceStore) GetAll() []*stnrv1a1.StaticService {
	ret := make([]*stnrv1a1.StaticService, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*stnrv1a1.StaticService)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global StaticServiceStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named StaticService object from the global storage
func (s *StaticServiceStore) GetObject(nsName types.NamespacedName) *stnrv1a1.StaticService {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*stnrv1a1.StaticService)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global StaticServiceStore")
	}

	return r
}

// // AddStaticService adds a StaticService object to the the global storage (this is used mainly for testing)
// func (s *StaticServiceStore) AddStaticService(gc *stnrv1a1.StaticService) {
// 	s.Upsert(gc)
// }
