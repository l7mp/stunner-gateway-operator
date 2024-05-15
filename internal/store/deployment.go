package store

import (
	appv1 "k8s.io/api/apps/v1"

	"k8s.io/apimachinery/pkg/types"
)

var Deployments = NewDeploymentStore()

type DeploymentStore struct {
	Store
}

func NewDeploymentStore() *DeploymentStore {
	return &DeploymentStore{
		Store: NewStore(),
	}
}

// GetAll returns all Deployment objects from the global storage
func (s *DeploymentStore) GetAll() []*appv1.Deployment {
	ret := make([]*appv1.Deployment, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*appv1.Deployment)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global DeploymentStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Deployment object from the global storage
func (s *DeploymentStore) GetObject(nsName types.NamespacedName) *appv1.Deployment {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*appv1.Deployment)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global DeploymentStore")
	}

	return r
}

func (s *DeploymentStore) DeepCopy() *DeploymentStore {
	ret := NewDeploymentStore()
	for _, o := range s.GetAll() {
		ret.Upsert(o.DeepCopy())
	}
	return ret
}
