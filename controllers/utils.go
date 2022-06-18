package controllers

import (
// "fmt"

// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// "sigs.k8s.io/controller-runtime/pkg/client"
// "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// func setObjectOwner(owner client.Object, obj client.Object) {
// 	foundOwnerRef := false
// 	for _, ownerRef := range obj.GetOwnerReferences() {
// 		if ownerRef.UID == owner.GetUID() {
// 			foundOwnerRef = true
// 		}
// 	}
// 	if !foundOwnerRef {
// 		obj.SetOwnerReferences(append(obj.GetOwnerReferences(), createObjectOwnerRef(owner)))
// 	}
// }

// func createObjectOwnerRef(obj client.Object) metav1.OwnerReference {
// 	return metav1.OwnerReference{
// 		APIVersion: getObjectAPIVersion(obj),
// 		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
// 		Name:       obj.GetName(),
// 		UID:        obj.GetUID(),
// 	}
// }

// func getObjectAPIVersion(obj client.Object) string {
// 	return fmt.Sprintf("%s/%s", obj.GetObjectKind().GroupVersionKind().Group, obj.GetObjectKind().GroupVersionKind().Version)
// }

// func compareObjectMetas(meta1 *metav1.ObjectMeta, meta2 *metav1.ObjectMeta) bool {
// 	return meta1.Namespace == meta2.Namespace &&
// 		meta1.Name == meta2.Name &&
// 		meta1.Generation == meta2.Generation
// }
