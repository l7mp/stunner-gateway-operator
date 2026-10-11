package store

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
)

func getAllSecrets(s Store) []*corev1.Secret {
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

func getSecret(s Store, nsName types.NamespacedName) *corev1.Secret {
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

// TLSSecrets holds the Gateway TLS certificates.
var TLSSecrets = NewTLSSecretStore()

type TLSSecretStore struct {
	Store
}

func NewTLSSecretStore() *TLSSecretStore {
	return &TLSSecretStore{
		Store: NewStore(),
	}
}

func (s *TLSSecretStore) GetAll() []*corev1.Secret { return getAllSecrets(s) }

func (s *TLSSecretStore) GetObject(nsName types.NamespacedName) *corev1.Secret {
	return getSecret(s, nsName)
}

// Authentication secrets for GatewayConfigs
var AuthSecrets = NewAuthSecretStore()

// TunnelAuthSecrets holds the Secrets the tunnels of TURNRoutes authenticate with.
var TunnelAuthSecrets = NewAuthSecretStore()

type AuthSecretStore struct {
	Store
}

func NewAuthSecretStore() *AuthSecretStore {
	return &AuthSecretStore{
		Store: NewStore(),
	}
}

func (s *AuthSecretStore) GetAll() []*corev1.Secret { return getAllSecrets(s) }

func (s *AuthSecretStore) GetObject(nsName types.NamespacedName) *corev1.Secret {
	return getSecret(s, nsName)
}
