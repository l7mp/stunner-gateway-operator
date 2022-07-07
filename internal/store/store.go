package store

import (
	"fmt"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Store interface {
	// Get returns an object from the store
	Get(nsName types.NamespacedName) client.Object
	// UpsertIfChanged adds the resource to the store and returns true if an actual update has
	// happened
	UpsertIfChanged(object client.Object) bool
	// Upsert adds the resource to the store
	Upsert(object client.Object)
	// Remove deletes an object from the store
	Remove(nsName types.NamespacedName)
	// Len returns the number of objects in the store
	Len() int
	// Objects returns all stored objects
	Objects() []client.Object
	// Flush empties the store
	Flush()
	// String returns a string with the keys of all stored objects
	String() string
}

type storeImpl struct {
	lock    sync.RWMutex
	objects map[string]client.Object
	// log     logr.Logger
}

// NewStore creates a new local object storage
func NewStore() Store {
	return &storeImpl{
		objects: make(map[string]client.Object),
	}
}

func (s *storeImpl) Get(nsName types.NamespacedName) client.Object {
	// s.log.V(3).Info("get", "key", nsName)
	s.lock.RLock()
	o, found := s.objects[nsName.String()]
	s.lock.RUnlock()

	if !found {
		// s.log.V(4).Info("get", "key", nsName, "result", "not-found")
		return nil
	}

	// s.log.V(4).Info("get", "key", nsName, "result", GetObjectKey(o))
	return o
}

func (s *storeImpl) UpsertIfChanged(new client.Object) bool {
	// s.log.V(3).Info("upsert", "key", GetObjectKey(new))
	key := GetObjectKey(new)

	s.lock.RLock()
	old, found := s.objects[key]
	s.lock.RUnlock()

	if found && compareObjects(old, new) == true {
		// s.log.V(4).Info("upsert", "key", GetObjectKey(new), "status", "unchanged")
		return false
	}

	s.Upsert(new)

	// s.log.V(4).Info("upsert", "key", GetObjectKey(new), "status", "new/changed")

	return true
}

func (s *storeImpl) Upsert(new client.Object) {
	// s.log.V(3).Info("upsert", "key", GetObjectKey(new))
	key := GetObjectKey(new)

	// lock for writing
	s.lock.Lock()
	defer s.lock.Unlock()
	s.objects[key] = new

	// s.log.V(4).Info("upsert", "key", GetObjectKey(new))
}

func (s *storeImpl) Remove(nsName types.NamespacedName) {
	// s.log.V(3).Info("remove", "key", nsName)
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

func (s *storeImpl) Flush() {
	os := s.Objects()
	for _, o := range os {
		n := types.NamespacedName{Namespace: o.GetNamespace(), Name: o.GetName()}
		s.Remove(n)
	}
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
