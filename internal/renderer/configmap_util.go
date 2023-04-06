package renderer

import (
	"encoding/json"
	// "fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	opdefault "github.com/l7mp/stunner-gateway-operator/api/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) renderConfig(c *RenderContext, name string, conf *stnrconfv1a1.StunnerConfig) (*corev1.ConfigMap, error) {
	s := ""

	if conf != nil {
		sc, err := json.Marshal(*conf)
		if err != nil {
			r.log.Error(err, "error marshaling dataplane config to JSON", "configmap", conf)
			return nil, NewCriticalError(ConfigMapRenderingError)
		}
		s = string(sc)
	}

	immutable := true
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.gwConf.GetNamespace(),
			Annotations: map[string]string{
				opdefault.DefaultRelatedGatewayAnnotationKey: store.GetObjectKey(c.gc),
			},
		},
		Immutable: &immutable,
		Data: map[string]string{
			opdefault.DefaultStunnerdConfigfileName: s,
		},
	}

	if err := controllerutil.SetOwnerReference(c.gwConf, cm, r.scheme); err != nil {
		r.log.Error(err, "cannot set owner reference", "owner", store.GetObjectKey(c.gc),
			"reference", store.GetObjectKey(cm))
		return nil, NewCriticalError(ConfigMapRenderingError)
	}

	return cm, nil
}
