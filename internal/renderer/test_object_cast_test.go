package renderer

import (
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func asConfigMap(o client.Object) *corev1.ConfigMap {
	v, ok := o.(*corev1.ConfigMap)
	if !ok {
		return nil
	}

	return v
}

func asDeployment(o client.Object) *appv1.Deployment {
	v, ok := o.(*appv1.Deployment)
	if !ok {
		return nil
	}

	return v
}

func asDaemonSet(o client.Object) *appv1.DaemonSet {
	v, ok := o.(*appv1.DaemonSet)
	if !ok {
		return nil
	}

	return v
}
