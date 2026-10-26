package store

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func GetObjectKey(object client.Object) string {
	n := types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}
	return n.String()
}

func GetNamespacedName(object client.Object) types.NamespacedName {
	return types.NamespacedName(client.ObjectKeyFromObject(object))
}

// FIXME this is not safe against K8s changing the namespace-name separator
func GetNameFromKey(key string) types.NamespacedName {
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
func DumpObject(o client.Object) string {
	// default dump
	output := fmt.Sprintf("%#v", o)

	// copy
	ro := o.DeepCopyObject()

	switch ro := ro.(type) {
	case *gwapiv1.GatewayClass:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *gwapiv1.Gateway:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *stnrgwv1.UDPRoute:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *corev1.Service:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *appv1.Deployment:
		if json, err := json.Marshal(strip(ro).(*appv1.Deployment)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *stnrgwv1.GatewayConfig:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *stnrgwv1.StaticService:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *stnrgwv1.Dataplane:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	default:
		// this is not fatal
		return output
	}

	return output
}

// for UDPRoutes
func DumpParentRef(p *gwapiv1.ParentReference) string {
	g, k, ns, sn := "<NIL>", "<NIL>", "<NIL>", "<NIL>"
	if p.Group != nil {
		g = string(*p.Group)
	}

	if p.Kind != nil {
		k = string(*p.Kind)
	}

	if p.Namespace != nil {
		ns = string(*p.Namespace)
	}

	if p.SectionName != nil {
		sn = string(*p.SectionName)
	}

	return fmt.Sprintf("{Group: %s, Kind: %s, Namespace: %s, Name: %s, SectionName: %s}",
		g, k, ns, p.Name, sn)
}

func DumpBackendRef(b *stnrgwv1.BackendRef) string {
	g, k, ns := "<NIL>", "<NIL>", "<NIL>"
	if b.Group != nil {
		g = string(*b.Group)
	}

	if b.Kind != nil {
		k = string(*b.Kind)
	}

	if b.Namespace != nil {
		ns = string(*b.Namespace)
	}

	return fmt.Sprintf("{Group: %s, Kind: %s, Namespace: %s, Name: %s}",
		g, k, ns, b.Name)
}

func strip(o client.Object) client.Object {
	as := o.GetAnnotations()
	if _, ok := as["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		delete(as, "kubectl.kubernetes.io/last-applied-configuration")
		o.SetAnnotations(as)
	}
	o.SetManagedFields([]metav1.ManagedFieldsEntry{})
	return o
}

// IsReferenceService returns true of the provided BackendRef points to a Service.
func IsReferenceService(ref *stnrgwv1.BackendRef) bool {
	// Group is the group of the referent. For example, "gateway.networking.k8s.io". When
	// unspecified or empty string, core API group is inferred.
	if ref.Group != nil && *ref.Group != corev1.GroupName {
		return false
	}

	if ref.Kind != nil && *ref.Kind != "Service" {
		return false
	}

	return true
}

// IsReferenceStaticService returns true of the provided BackendRef points to a StaticService.
func IsReferenceStaticService(ref *stnrgwv1.BackendRef) bool {
	if ref.Group == nil || string(*ref.Group) != stnrgwv1.GroupVersion.Group {
		return false
	}

	if ref.Kind == nil || (*ref.Kind) != "StaticService" {
		return false
	}

	return true
}

// taken from redhat operator-utils: https://github.com/redhat-cop/operator-utils/blob/master/pkg/util/owner.go
func IsOwner(owner, owned metav1.Object, kind string) bool {
	for _, ownerRef := range owned.GetOwnerReferences() {
		if ownerRef.Name == owner.GetName() && ownerRef.UID == owner.GetUID() &&
			ownerRef.Kind == kind {
			return true
		}
	}

	return false
}

// MergeMetadata merges labels or annotations. If conflict, the label/annotation in the second
// argument overrrides the first one. Returns a new map to avoid unintentional sharing.
func MergeMetadata(a, b map[string]string) map[string]string {
	ret := make(map[string]string)
	for k, v := range a {
		ret[k] = v
	}
	for k, v := range b {
		ret[k] = v
	}

	return ret
}

// FormatManagedKeys renders the keys of the labels or annotations the operator adds into the
// value of the managed-labels or managed-annotations annotation: sorted and comma-separated.
func FormatManagedKeys(m map[string]string) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ",")
}

// PruneManagedKeys deletes from m the keys the managed-labels or managed-annotations annotation
// value old lists and new does not: the keys the operator added earlier and no longer adds. Keys
// set by other actors are never listed, so they stay.
func PruneManagedKeys(m map[string]string, old, new string) {
	keep := strings.Split(new, ",")
	for _, k := range strings.Split(old, ",") {
		if !slices.Contains(keep, k) {
			delete(m, k)
		}
	}
}

// FilterLabels returns a copy of in with any key in filter removed. Each removed key is logged
// at V(1) so operators can confirm the filter is firing. The input map is not mutated. If either
// in or filter is empty, the input is returned unchanged.
func FilterLabels(in map[string]string, filter []string, log logr.Logger) map[string]string {
	if len(in) == 0 || len(filter) == 0 {
		return in
	}
	drop := make(map[string]struct{}, len(filter))
	for _, k := range filter {
		drop[k] = struct{}{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if _, hit := drop[k]; hit {
			log.V(1).Info("filtering label from propagation", "key", k, "value", v)
			continue
		}
		out[k] = v
	}

	return out
}
