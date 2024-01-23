package store

import (
	"k8s.io/apimachinery/pkg/types"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

var UDPRoutes = NewUDPRouteStore()
var UDPRoutesV1A2 = NewUDPRouteStore()

type UDPRouteStore struct {
	Store
}

func NewUDPRouteStore() *UDPRouteStore {
	return &UDPRouteStore{
		Store: NewStore(),
	}
}

// GetAll returns all UDPRoute objects from the global storage
func (s *UDPRouteStore) GetAll() []*stnrgwv1.UDPRoute {
	ret := make([]*stnrgwv1.UDPRoute, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*stnrgwv1.UDPRoute)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global UDPRouteStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named UDPRoute object from the global storage
func (s *UDPRouteStore) GetObject(nsName types.NamespacedName) *stnrgwv1.UDPRoute {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*stnrgwv1.UDPRoute)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global UDPRouteStore")
	}

	return r
}

func (s *UDPRouteStore) DeepCopy() *UDPRouteStore {
	ret := NewUDPRouteStore()
	for _, o := range s.GetAll() {
		ret.Upsert(o)
	}
	return ret
}
