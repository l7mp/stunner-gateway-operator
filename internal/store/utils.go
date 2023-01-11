package store

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetObjectKey(object client.Object) string {
	// s.log.V(5).Info("GetObjectKey", "object", fmt.Sprintf("%s/%s", object.GetNamespace(), object.GetName()))

	n := types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}
	return n.String()
}

func GetNamespacedName(object client.Object) types.NamespacedName {
	// s.log.V(5).Info("GetObjectKey", "object", fmt.Sprintf("%s/%s", object.GetNamespace(), object.GetName()))

	return types.NamespacedName(client.ObjectKeyFromObject(object))
}

// FIXME this is not safe against K8s changing the namespace-name separator
func GetNameFromKey(key string) types.NamespacedName {
	// s.log.V(5).Info("GetNameFromKey", "key", key)

	ns := strings.SplitN(key, "/", 2)
	return types.NamespacedName{Namespace: ns[0], Name: ns[1]}
}

// Two resources are different if:
// (1) They have different namespaces or names.
// (2) They have the same namespace and name (resources are the same resource) but their specs are different.
// If their specs are different, their Generations are different too. So we only test their Generations.
// note: annotations are not part of the spec, so their update doesn't affect the Generation.
func compareObjects(o1, o2 client.Object) bool {
	return o1.GetNamespace() == o2.GetNamespace() &&
		o1.GetName() == o2.GetName() &&
		o1.GetGeneration() == o2.GetGeneration()
}

// DumpObject convers an object into a human-readable form for logging.
func DumpObject(conf interface{}) string {
	output := fmt.Sprintf("%#v", conf)
	if json, err := json.Marshal(conf); err == nil {
		output = string(json)
	}
	return output
}
