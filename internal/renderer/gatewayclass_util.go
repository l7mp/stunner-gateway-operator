package renderer

import (
	"fmt"
	// "github.com/go-logr/logr"
	// apiv1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	// ctlr "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/manager" corev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerctrl "github.com/l7mp/stunner-gateway-operator/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
)

func (r *Renderer) getGatewayClass() (*gatewayv1alpha2.GatewayClass, error) {
	r.log.V(4).Info("getGatewayClass")
	o := r.op

	gcs, err := o.GetGatewayClasses()
	if err != nil {
		return nil, err
	}

	if len(gcs) == 0 {
		return nil, fmt.Errorf("no GatewayClass found")
	}

	if len(gcs) > 1 {
		return nil, fmt.Errorf("too many GatewayClass objects %d: "+
			"refusing to render STUNner configuration, ", len(gcs))
	}

	// play it safe
	gc := gcs[0]
	if string(gc.Spec.ControllerName) != o.GetControllerName() {
		return nil, fmt.Errorf("invalid gateway: unknown controller controller-name %q, "+
			"expecting %q", string(gc.Spec.ControllerName), o.GetControllerName())
	}

	// this should already be validated but play it safe
	ref := gc.Spec.ParametersRef
	if ref == nil {
		return nil, fmt.Errorf("empty ParametersRef in GatewayClassSpec: %#v", gc.Spec)
	}

	if ref.Group != gatewayv1alpha2.Group(stunnerv1alpha1.GroupVersion.Group) {
		return nil, fmt.Errorf("invalid Group in GatewayClassSpec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Name == "" {
		return nil, fmt.Errorf("empty name in GatewayClassSpec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Namespace == nil || (ref.Namespace != nil && *ref.Namespace == "") {
		return nil, fmt.Errorf("empty namespace in GatewayClassSpec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Kind != gatewayv1alpha2.Kind("GatewayConfig") {
		return nil, fmt.Errorf("expecting ParametersRef to point to a GatewayConfig "+
			"resource: %#v", *gc.Spec.ParametersRef)
	}

	r.log.V(2).Info("getGatewayClass", "result", store.GetObjectKey(gc))

	return gc, nil
}

func setGatewayClassStatusScheduled(gc *gatewayv1alpha2.GatewayClass, cname string) {
	meta.SetStatusCondition(&gc.Status.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.GatewayConditionScheduled),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gc.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.GatewayReasonScheduled),
		Message:            fmt.Sprintf("gatewayclass under processing by controller %q", cname),
	})
}

func setGatewayClassStatusReady(gc *gatewayv1alpha2.GatewayClass, cname string) {
	meta.SetStatusCondition(&gc.Status.Conditions, metav1.Condition{
		Type:               string(gatewayv1alpha2.GatewayConditionReady),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: gc.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             string(gatewayv1alpha2.GatewayReasonReady),
		Message:            fmt.Sprintf("gateway is now managed by controller %q", cname),
	})
}
