package store

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
)

var ConfigMaps = NewConfigMapStore()

type ConfigMapStore struct {
	Store
}

func NewConfigMapStore() *ConfigMapStore {
	return &ConfigMapStore{
		Store: NewStore(),
	}
}

// GetAll returns all ConfigMap objects from the global storage
func (s *ConfigMapStore) GetAll() []*corev1.ConfigMap {
	ret := make([]*corev1.ConfigMap, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*corev1.ConfigMap)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global ConfigMapStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named ConfigMap object from the global storage
func (s *ConfigMapStore) GetObject(nsName types.NamespacedName) *corev1.ConfigMap {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*corev1.ConfigMap)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global ConfigMapStore")
	}

	return r
}

// // AddConfigMap adds a ConfigMap object to the the global storage (this is used mainly for testing)
// func (s *ConfigMapStore) AddConfigMap(gc *corev1.ConfigMap) {
// 	s.Upsert(gc)
// }
