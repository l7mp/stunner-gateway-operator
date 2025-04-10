package store

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"
)

var Nodes = NewNodeStore()

type NodeStore struct {
	Store
}

func NewNodeStore() *NodeStore {
	return &NodeStore{
		Store: NewStore(),
	}
}

// GetAll returns all Node objects from the global storage
func (s *NodeStore) GetAll() []*corev1.Node {
	ret := make([]*corev1.Node, 0)

	objects := s.Objects()
	for i := range objects {
		r, ok := objects[i].(*corev1.Node)
		if !ok {
			// this is critical: throw up hands and die
			panic("access to an invalid object in the global NodeStore")
		}

		ret = append(ret, r)
	}

	return ret
}

// GetObject returns a named Node object from the global storage
func (s *NodeStore) GetObject(nsName types.NamespacedName) *corev1.Node {
	o := s.Get(nsName)
	if o == nil {
		return nil
	}

	r, ok := o.(*corev1.Node)
	if !ok {
		// this is critical: throw up hands and die
		panic("access to an invalid object in the global NodeStore")
	}

	return r
}

// GetExternalAddress returns the first external IP or DNS address of a node
func GetExternalAddress(n *corev1.Node) (corev1.NodeAddressType, string, bool) {
	// find an external IP
	ok := false
	addr := ""
	aType := corev1.NodeAddressType("")
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeExternalIP {
			addr = a.Address
			aType = a.Type
			ok = true
			break
		}
	}

	// if no external IP, try to find an external DNS address
	if addr == "" {
		for _, a := range n.Status.Addresses {
			if a.Type == corev1.NodeExternalDNS {
				addr = a.Address
				aType = a.Type
				ok = true
				break
			}
		}
	}

	return aType, addr, ok
}
