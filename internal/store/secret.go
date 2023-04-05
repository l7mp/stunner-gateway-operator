package store

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
)

var Secrets = NewSecretStore()

type SecretStore struct {
	Store
}

func NewSecretStore() *SecretStore {
	return &SecretStore{
		Store: NewStore(),
	}
}

// GetAll returns all Secret objects from the global storage
func (s *SecretStore) GetAll() []*corev1.Secret {
	ret := make([]*corev1.Secret, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*corev1.Secret)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global SecretStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Secret object from the global storage
func (s *SecretStore) GetObject(nsName types.NamespacedName) *corev1.Secret {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*corev1.Secret)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global SecretStore")
	}

	return r
}

// // AddSecret adds a Secret object to the the global storage (this is used mainly for testing)
// func (s *SecretStore) AddSecret(gc *corev1.Secret) {
// 	s.Upsert(gc)
// }

var AuthSecrets = NewAuthSecretStore()

type AuthSecretStore struct {
	Store
}

func NewAuthSecretStore() *AuthSecretStore {
	return &AuthSecretStore{
		Store: NewStore(),
	}
}

// GetAll returns all AuthSecret objects from the global storage.
func (s *AuthSecretStore) GetAll() []*corev1.Secret {
	ret := make([]*corev1.Secret, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*corev1.Secret)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global AuthSecretStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named AuthSecret object from the global storage
func (s *AuthSecretStore) GetObject(nsName types.NamespacedName) *corev1.Secret {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*corev1.Secret)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global AuthSecretStore")
	}

	return r
}

// // AddAuthSecret adds a AuthSecret object to the the global storage (this is used mainly for testing)
// func (s *AuthSecretStore) AddAuthSecret(gc *corev1.AuthSecret) {
// 	s.Upsert(gc)
// }
