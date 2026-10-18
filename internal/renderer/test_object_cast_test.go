package renderer

import (
	appv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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
