package renderer

import (
	"encoding/json"
	// "fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func (r *Renderer) renderConfig(c *RenderContext, name, namespace string, conf *stnrconfv1.StunnerConfig) (*corev1.ConfigMap, error) {
	s := ""

	if conf != nil {
		sc, err := json.Marshal(*conf)
		if err != nil {
			r.log.Error(err, "Error marshaling dataplane config to JSON", "configmap", conf)
			return nil, NewCriticalError(RenderingError)
		}
		s = string(sc)
	}

	relatedGateway := store.GetObjectKey(c.gc)
	if config.DataplaneMode == config.DataplaneModeManaged {
		gw := c.gws.GetFirst()
		if gw == nil {
			r.log.Info("Internal error: config renderer called with empty Gateway ref in managed mode")
			return nil, NewCriticalError(RenderingError)
		}
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
				opdefault.RelatedGatewayKey: relatedGateway,
			},
		},
		Immutable: &immutable,
		Data: map[string]string{
			opdefault.DefaultStunnerdConfigfileName: s,
		},
	}

	// owned by the gateway-config in legacy mode
	var owner client.Object = c.gwConf
	if config.DataplaneMode == config.DataplaneModeManaged {
		gw := c.gws.GetFirst()
		if gw == nil {
			panic("renderConfig called with empty Gateway ref in managed mode")
		}
		owner = gw

		// add also the missing labels
		labels := cm.GetLabels()
		labels[opdefault.RelatedGatewayKey] = gw.GetName()
		labels[opdefault.RelatedGatewayNamespace] = gw.GetNamespace()
		cm.SetLabels(labels)
	}

	if err := controllerutil.SetOwnerReference(owner, cm, r.scheme); err != nil {
		r.log.Error(err, "Cannot set owner reference", "owner", store.GetObjectKey(owner),
			"reference", store.GetObjectKey(cm))
		return nil, NewCriticalError(RenderingError)
	}

	return cm, nil
}
