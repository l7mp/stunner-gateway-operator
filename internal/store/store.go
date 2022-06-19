package store

import (
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Store interface {
	// Get returns an object from the store
	Get(nsName types.NamespacedName) client.Object
	// Upsert adds the resource to the store and returns true if an actual update has happened
	Upsert(object client.Object) bool
	// Remove deletes an object from the store
	Remove(nsName types.NamespacedName)
	// Len returns the number of objects in the store
	Len() int
	// Objects returns all stored objects
	Objects() []client.Object
	// String returns a string with the keys of all stored objects
	String() string
}

type storeImpl struct {
	lock    sync.RWMutex
	objects map[string]client.Object
	log     logr.Logger
}

// NewStore creates a new local object storage
func NewStore(logger logr.Logger) Store {
	return &storeImpl{
		objects: make(map[string]client.Object),
		log:     logger,
	}
}

func (s *storeImpl) Get(nsName types.NamespacedName) client.Object {
	s.log.V(3).Info("get", "key", nsName)
	s.lock.RLock()
	o, found := s.objects[nsName.String()]
	s.lock.RUnlock()

	if !found {
		s.log.V(4).Info("get", "key", nsName, "result", "not-found")
		return nil
	}

	s.log.V(4).Info("get", "key", nsName, "result", GetObjectKey(o))
	return o
}

func (s *storeImpl) Upsert(new client.Object) bool {
	s.log.V(3).Info("upsert", "key", GetObjectKey(new))
	key := GetObjectKey(new)

	s.lock.RLock()
	old, found := s.objects[key]
	s.lock.RUnlock()

	if found && compareObjects(old, new) == true {
		s.log.V(4).Info("upsert", "key", GetObjectKey(new), "status", "unchanged")
		return false
	}

	// lock for writing
	s.lock.Lock()
	defer s.lock.Unlock()
	s.objects[key] = new

	s.log.V(4).Info("upsert", "key", GetObjectKey(new), "status", "new/changed")

	return true
}

func (s *storeImpl) Remove(nsName types.NamespacedName) {
	s.log.V(3).Info("remove", "key", nsName)
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.objects, nsName.String())
}

//FIXME is length(map) atomic in Go? play it safe...
func (s *storeImpl) Len() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.objects)
}

func (s *storeImpl) Objects() []client.Object {
	s.lock.RLock()
	defer s.lock.RUnlock()

	ret := make([]client.Object, s.Len())
	i := 0
	for _, o := range s.objects {
		ret[i] = o
		i += 1
	}

	return ret
}

func (s *storeImpl) String() string {
	os := s.Objects()
	ret := []string{}
	for _, o := range os {
		ret = append(ret, GetObjectKey(o))
	}
	return fmt.Sprintf("store (%d objects): %s", len(os),
		strings.Join(ret, ", "))
}
