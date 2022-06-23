package store

import (
	"k8s.io/apimachinery/pkg/types"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var UDPRoutes = NewUDPRouteStore()

type UDPRouteStore struct {
	Store
}

func NewUDPRouteStore() *UDPRouteStore {
	return &UDPRouteStore{
		Store: NewStore(),
	}
}

// GetAll returns all UDPRoute objects from the global storage
func (s *UDPRouteStore) GetAll() []*gatewayv1alpha2.UDPRoute {
	ret := make([]*gatewayv1alpha2.UDPRoute, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*gatewayv1alpha2.UDPRoute)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global UDPRouteStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named UDPRoute object from the global storage
func (s *UDPRouteStore) GetObject(nsName types.NamespacedName) *gatewayv1alpha2.UDPRoute {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*gatewayv1alpha2.UDPRoute)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global UDPRouteStore")
	}

	return r
}

// // AddUDPRoute adds a UDPRoute object to the the global storage (this is used mainly for testing)
// func (s *UDPRouteStore) AddUDPRoute(gc *gatewayv1alpha2.UDPRoute) {
// 	s.Upsert(gc)
// }
