package renderer

import (
	"fmt"
	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/types"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) getGatewayConfig4Class(c *RenderContext) (*stunnerv1alpha1.GatewayConfig, error) {
	r.log.V(4).Info("getGatewayConfig4Class", "gateway-class", store.GetObjectKey(c.gc))

	// ref already checked
	ref := c.gc.Spec.ParametersRef

	gwConfName := types.NamespacedName{
		Namespace: string(*ref.Namespace), // this should already be validated
		Name:      ref.Name,
	}

	gwConf := store.GatewayConfigs.GetObject(gwConfName)
	if gwConf == nil {
		return nil, fmt.Errorf("cannot find gateway-config in parametersRef with name: %s",
			gwConfName.String())
	}

	r.log.V(4).Info("getGatewayConfig4Class", "gateway-class", store.GetObjectKey(c.gc), "result",
		store.GetObjectKey(gwConf))

	return gwConf, nil
}
