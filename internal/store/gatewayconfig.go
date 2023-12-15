package store

import (
	"k8s.io/apimachinery/pkg/types"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

var GatewayConfigs = NewGatewayConfigStore()

type GatewayConfigStore struct {
	Store
}

func NewGatewayConfigStore() *GatewayConfigStore {
	return &GatewayConfigStore{
		Store: NewStore(),
	}
}

// GetAll returns all GatewayConfig objects from the global storage
func (s *GatewayConfigStore) GetAll() []*stnrgwv1.GatewayConfig {
	ret := make([]*stnrgwv1.GatewayConfig, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*stnrgwv1.GatewayConfig)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global GatewayConfigStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named GatewayConfig object from the global storage
func (s *GatewayConfigStore) GetObject(nsName types.NamespacedName) *stnrgwv1.GatewayConfig {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*stnrgwv1.GatewayConfig)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global GatewayConfigStore")
	}

	return r
}

// // AddGatewayConfig adds a GatewayConfig object to the the global storage (this is used mainly for testing)
// func (s *GatewayConfigStore) AddGatewayConfig(gc *stnrgwv1.GatewayConfig) {
// 	s.Upsert(gc)
// }
