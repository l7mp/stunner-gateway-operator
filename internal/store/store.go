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
	// Set resets the store to the specified objects
	Reset(objects []client.Object)
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

// Merge merges a store with another one.
func Merge(dst, src Store) {
	for _, o := range src.Objects() {
		dst.Upsert(o)
	}
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
	s.lock.RLock()
	o, found := s.objects[nsName.String()]
	s.lock.RUnlock()

	if !found {
		return nil
	}

	return o
}

// Reset resets a store from a list of objects and removes duplicates along the way.
func (s *storeImpl) Reset(objects []client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for k := range s.objects {
		delete(s.objects, k)
	}

	for _, o := range objects {
		s.objects[GetObjectKey(o)] = o
	}
}

func (s *storeImpl) UpsertIfChanged(new client.Object) bool {
	key := GetObjectKey(new)

	s.lock.RLock()
	old, found := s.objects[key]
	s.lock.RUnlock()

	if found && compareObjects(old, new) {
		return false
	}

	s.Upsert(new)

	return true
}

func (s *storeImpl) Upsert(new client.Object) {
	key := GetObjectKey(new)

	// lock for writing
	s.lock.Lock()
	defer s.lock.Unlock()
	s.objects[key] = new
}

func (s *storeImpl) Remove(nsName types.NamespacedName) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.objects, nsName.String())
}

// FIXME is length(map) atomic in Go? play it safe...
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
