package renderer

import (
	"encoding/json"
	// "fmt"
	// "reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "sigs.k8s.io/controller-runtime/pkg/client"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
	// gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
)

// // set owner ref without using the scheme:
// func setOwner2GatewayConfig(gc, object client.Object) {
// 	gvk := stunnerv1alpha1.GroupVersion.WithKind("GatewayConfig")
// 	ref := metav1.OwnerReference{
// 		APIVersion: gvk.GroupVersion().String(),
// 		Kind:       gvk.Kind,
// 		UID:        gc.GetUID(),
// 		Name:       gc.GetName(),
// 	}

// 	owners := object.GetOwnerReferences()
// 	if idx := indexOwnerRef(owners, ref); idx == -1 {
// 		owners = append(owners, ref)
// 	} else {
// 		owners[idx] = ref
// 	}
// 	object.SetOwnerReferences(owners)
// }

func indexOwnerRef(ownerReferences []metav1.OwnerReference, ref metav1.OwnerReference) int {
	for index, r := range ownerReferences {
		if r.Kind == ref.Kind && r.Name == ref.Name {
			return index
		}
	}
	return -1
}

func (r *Renderer) write2ConfigMap(ns, name string, conf *stunnerconfv1alpha1.StunnerConfig) (*corev1.ConfigMap, error) {
	s := ""

	if conf != nil {
		sc, err := json.Marshal(*conf)
		if err != nil {
			return nil, err
		} else {
			s = string(sc)
		}
	}

	immutable := true

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Immutable: &immutable,
		Data: map[string]string{
			config.DefaultStunnerdConfigfileName: s,
		},
	}, nil
}
