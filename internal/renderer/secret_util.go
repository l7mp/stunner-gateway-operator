package renderer

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// getSecretNameFromRef obtains a namespaced name from a SecretObjectReference, performing validity
// checks along the way.
func getSecretNameFromRef(ref *gwapiv1.SecretObjectReference, namespace string) (types.NamespacedName, error) {
	ret := types.NamespacedName{}
	if ref == nil {
		return ret, errors.New("Internal error obtaining Secret: called with nil pointer")
	}

	// - group MUST be set to "" (corev1.GroupName), "v1", or omitted,
	if ref.Group != nil && (string(*ref.Group) != corev1.GroupName && string(*ref.Group) != "v1") {
		return ret, fmt.Errorf("Invalid Secret reference: invalid Group")
	}

	// - kind MUST be set to "Secret" or omitted,
	if ref.Kind != nil && string(*ref.Kind) != "Secret" {
		return ret, fmt.Errorf("Invalid Secret reference: invalid Kind")
	}

	// - namespace MAY be omitted, in which case it defaults to the given namespace, or it
	//   MAY be any valid namespace where the Secret lives.
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	ret = types.NamespacedName{Namespace: namespace, Name: string(ref.Name)}
	return ret, nil
}

// dumpSecretRef is a helper to create a human-readable dump from a secret ref.
func dumpSecretRef(ref *gwapiv1.SecretObjectReference, namespace string) string {
	if ref == nil {
		return "<nil>"
	}

	group := "<nil>"
	if ref.Group != nil {
		group = string(*ref.Group)
	}

	kind := "<nil>"
	if ref.Kind != nil {
		kind = string(*ref.Kind)
	}

	// - namespace MAY be omitted, in which case it defaults to the given namespace, or it
	//   MAY be any valid namespace where the Secret lives.
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	return fmt.Sprintf("{Group: %s, Kind: %s, Namespace: %s, Name: %s}", group, kind,
		namespace, string(ref.Name))
}
