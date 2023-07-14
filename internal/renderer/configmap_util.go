package renderer

import (
	"encoding/json"
	// "fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func (r *Renderer) renderConfig(c *RenderContext, name, namespace string, conf *stnrconfv1a1.StunnerConfig) (*corev1.ConfigMap, error) {
	s := ""

	if conf != nil {
		sc, err := json.Marshal(*conf)
		if err != nil {
			r.log.Error(err, "error marshaling dataplane config to JSON", "configmap", conf)
			return nil, NewCriticalError(RenderingError)
		}
		s = string(sc)
	}

	relatedGateway := store.GetObjectKey(c.gc)
	if config.DataplaneMode == config.DataplaneModeManaged {
		gw := c.gws.GetFirst()
		relatedGateway = store.GetObjectKey(gw)
	}

	immutable := true
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayAnnotationKey: relatedGateway,
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
		return nil, NewCriticalError(RenderingError)
	}

	return cm, nil
}
