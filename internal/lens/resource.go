package lens

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func projectMetadata(current, owned client.Object) metav1.ObjectMeta {
	v := metav1.ObjectMeta{
		Name:        current.GetName(),
		Namespace:   current.GetNamespace(),
		Labels:      projectOwnedMap(current.GetLabels(), owned.GetLabels()),
		Annotations: projectOwnedMap(current.GetAnnotations(), owned.GetAnnotations()),
	}

	v.OwnerReferences = projectOwnedOwnerReferences(current.GetOwnerReferences(), owned.GetOwnerReferences())

	return v
}

func projectOwnedMap(current, owned map[string]string) map[string]string {
	if len(owned) == 0 {
		return nil
	}

	ret := make(map[string]string, len(owned))
	for k := range owned {
		if v, ok := current[k]; ok {
			ret[k] = v
		}
	}

	if len(ret) == 0 {
		return nil
	}

	return ret
}

func projectOwnedOwnerReferences(current, owned []metav1.OwnerReference) []metav1.OwnerReference {
	if len(owned) == 0 {
		return nil
	}

	ret := make([]metav1.OwnerReference, 0, len(owned))
	for i := range owned {
		for j := range current {
			if current[j].Name != owned[i].Name || current[j].Kind != owned[i].Kind {
				continue
			}

			ret = append(ret, metav1.OwnerReference{
				APIVersion: current[j].APIVersion,
				Kind:       current[j].Kind,
				Name:       current[j].Name,
				UID:        current[j].UID,
			})
			break
		}
	}

	if len(ret) == 0 {
		return nil
	}

	return ret
}

func setMetadata(dst, src client.Object) error {
	labs := store.MergeMetadata(dst.GetLabels(), src.GetLabels())
	dst.SetLabels(labs)

	annotations := store.MergeMetadata(dst.GetAnnotations(), src.GetAnnotations())
	dst.SetAnnotations(annotations)

	return addOwnerRef(dst, src)
}

func addOwnerRef(dst, src client.Object) error {
	ownerRefs := src.GetOwnerReferences()
	if len(ownerRefs) != 1 {
		return fmt.Errorf("addOwnerRef: expecting a singleton ownerRef in %q/%q, found %d",
			src.GetNamespace(), src.GetName(), len(ownerRefs))
	}
	ownerRef := src.GetOwnerReferences()[0]

	for i, ref := range dst.GetOwnerReferences() {
		if ref.Name == ownerRef.Name && ref.Kind == ownerRef.Kind {
			ownerRefs = dst.GetOwnerReferences()
			ownerRef.DeepCopyInto(&ownerRefs[i])
			dst.SetOwnerReferences(ownerRefs)

			return nil
		}
	}

	ownerRefs = dst.GetOwnerReferences()
	ownerRefs = append(ownerRefs, ownerRef)
	dst.SetOwnerReferences(ownerRefs)

	return nil
}

func projectTemplateMeta(current, owned *corev1.PodTemplateSpec) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Labels:      projectOwnedMap(current.GetLabels(), owned.GetLabels()),
		Annotations: projectOwnedMap(current.GetAnnotations(), owned.GetAnnotations()),
	}
}
