package lens

import (
	"fmt"
	"maps"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

type ConfigMapLens struct {
	corev1.ConfigMap `json:",inline"`
}

func NewConfigMapLens(cm *corev1.ConfigMap) *ConfigMapLens {
	return &ConfigMapLens{ConfigMap: *cm.DeepCopy()}
}

func (l *ConfigMapLens) EqualResource(current client.Object) bool {
	cm, ok := current.(*corev1.ConfigMap)
	if !ok {
		return false
	}

	return equality.Semantic.DeepEqual(projectConfigMap(cm, &l.ConfigMap), projectConfigMap(&l.ConfigMap, &l.ConfigMap))
}

func (l *ConfigMapLens) ApplyToResource(target client.Object) error {
	cm, ok := target.(*corev1.ConfigMap)
	if !ok {
		return fmt.Errorf("configmap lens: invalid target type %T", target)
	}

	if err := setMetadata(cm, &l.ConfigMap); err != nil {
		return err
	}

	projected := projectConfigMap(&l.ConfigMap, &l.ConfigMap)
	cm.Data = maps.Clone(projected.Data)
	return nil
}

func (l *ConfigMapLens) EqualStatus(_ client.Object) bool {
	return true
}

func (l *ConfigMapLens) ApplyToStatus(_ client.Object) error {
	return nil
}

func (l *ConfigMapLens) DeepCopy() *ConfigMapLens {
	return &ConfigMapLens{ConfigMap: *l.ConfigMap.DeepCopy()}
}

func (l *ConfigMapLens) DeepCopyObject() runtime.Object { return l.DeepCopy() }

func projectConfigMap(cm, owned *corev1.ConfigMap) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: projectMetadata(cm, owned),
		Data:       maps.Clone(cm.Data),
	}
}
