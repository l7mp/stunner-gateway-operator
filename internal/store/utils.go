package store

import (
	"encoding/json"
	"fmt"
	"strings"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
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

// unpacks a stunner config
func UnpackConfigMap(cm *corev1.ConfigMap) (stnrconfv1a1.StunnerConfig, error) {
	conf := stnrconfv1a1.StunnerConfig{}

	jsonConf, found := cm.Data[opdefault.DefaultStunnerdConfigfileName]
	if !found {
		return conf, fmt.Errorf("error unpacking configmap data: %s not found",
			opdefault.DefaultStunnerdConfigfileName)
	}

	if err := json.Unmarshal([]byte(jsonConf), &conf); err != nil {
		return conf, err
	}

	return conf, nil
}

// DumpObject convers an object into a human-readable form for logging.
func DumpObject(o client.Object) string {
	// default dump
	output := fmt.Sprintf("%#v", o)

	// copy
	ro := o.DeepCopyObject()

	switch ro := ro.(type) {
	case *gwapiv1b1.GatewayClass:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *gwapiv1b1.Gateway:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *gwapiv1a2.UDPRoute:
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
	case *stnrv1a1.GatewayConfig:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *stnrv1a1.StaticService:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *stnrv1a1.Dataplane:
		if json, err := json.Marshal(strip(ro)); err != nil {
			fmt.Printf("---------------ERROR-----------: %s\n", err)
		} else {
			output = string(json)
		}
	case *corev1.ConfigMap:
		if json, err := json.Marshal(strip(stripCM(ro))); err != nil {
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

// // DumpObject convers an object into a human-readable form for logging.
// func DumpObject(o client.Object) string {
// 	// default dump
// 	output := fmt.Sprintf("%#v", o)

// 	// copy
// 	ro := o.DeepCopyObject()

// 	var tmp client.Object
// 	switch ro := ro.(type) {
// 	case *gwapiv1b1.GatewayClass:
// 		tmp = ro
// 	case *gwapiv1b1.Gateway:
// 		tmp = ro
// 	case *gwapiv1a2.UDPRoute:
// 		tmp = ro
// 	case *corev1.Service:
// 		tmp = ro
// 	case *appv1.Deployment:
// 		tmp = ro
// 	case *stnrv1a1.GatewayConfig:
// 		tmp = ro
// 	case *stnrv1a1.StaticService:
// 		tmp = ro
// 	case *stnrv1a1.Dataplane:
// 		tmp = ro
// 	case *corev1.ConfigMap:
// 		tmp = stripCM(ro)
// 	default:
// 		// this is not fatal
// 		return output
// 	}

// 	// remove cruft
// 	tmp = strip(tmp)

// 	if json, err := json.Marshal(tmp); err != nil {
// 		fmt.Printf("---------------ERROR-----------: %s\n", err)
// 		return output
// 	} else {
// 		output = string(json)
// 	}
// 	return output
// }

func strip(o client.Object) client.Object {
	as := o.GetAnnotations()
	if _, ok := as["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		delete(as, "kubectl.kubernetes.io/last-applied-configuration")
		o.SetAnnotations(as)
	}
	o.SetManagedFields([]metav1.ManagedFieldsEntry{})
	return o
}

func stripCM(cm *corev1.ConfigMap) *corev1.ConfigMap {
	// remove keys from the config
	conf, err := UnpackConfigMap(cm)
	if err != nil {
		return cm
	}

	if _, ok := conf.Auth.Credentials["username"]; ok {
		conf.Auth.Credentials["username"] = "-SECRET-"
	}
	if _, ok := conf.Auth.Credentials["password"]; ok {
		conf.Auth.Credentials["password"] = "-SECRET-"
	}
	if _, ok := conf.Auth.Credentials["secret"]; ok {
		conf.Auth.Credentials["secret"] = "-SECRET-"
	}

	for i := range conf.Listeners {
		if conf.Listeners[i].Cert != "" {
			conf.Listeners[i].Cert = "-SECRET-"
		}
		if conf.Listeners[i].Key != "" {
			conf.Listeners[i].Key = "-SECRET-"
		}
	}

	sc, err := json.Marshal(conf)
	if err != nil {
		return cm
	}

	cm.Data = map[string]string{
		opdefault.DefaultStunnerdConfigfileName: string(sc),
	}

	return cm
}

// IsReferenceService returns true of the provided BackendRef points to a Service.
func IsReferenceService(ref *gwapiv1b1.BackendRef) bool {
	// Group is the group of the referent. For example, “gateway.networking.k8s.io”. When
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
func IsReferenceStaticService(ref *gwapiv1b1.BackendRef) bool {
	if ref.Group == nil || string(*ref.Group) != stnrv1a1.GroupVersion.Group {
		return false
	}

	if ref.Kind == nil || (*ref.Kind) != "StaticService" {
		return false
	}

	return true
}

// taken from redhat operator-utils: https://github.com/redhat-cop/operator-utils/blob/master/pkg/util/owner.go
func IsOwner(owner, owned metav1.Object, kind string) bool {
	// fmt.Println("-------------------------")
	// fmt.Printf("%#v\n", owner)
	// fmt.Printf("%#v\n", owned)
	// fmt.Println("-------------------------")
	for _, ownerRef := range owned.GetOwnerReferences() {
		if ownerRef.Name == owner.GetName() && ownerRef.UID == owner.GetUID() &&
			ownerRef.Kind == kind {
			return true
		}
	}

	return false
}
