package renderer

import (
	"fmt"

	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) getGatewayClasses() []*gwapiv1a2.GatewayClass {
	// r.log.V(4).Info("getGatewayClasses")
	ret := []*gwapiv1a2.GatewayClass{}

	for _, gc := range store.GatewayClasses.GetAll() {
		if err := r.validateGatewayClass(gc); err != nil {
			r.log.Error(err, "invalid gateway-class", "gateway-class", store.GetObjectKey(gc))
			continue
		}

		ret = append(ret, gc)
	}

	r.log.V(2).Info("getGatewayClasses", "found", fmt.Sprintf("%d gateway-classes", len(ret)))

	return ret
}

func (r *Renderer) validateGatewayClass(gc *gwapiv1a2.GatewayClass) error {
	// r.log.V(4).Info("validateGatewayClass")

	// play it safe
	if string(gc.Spec.ControllerName) != config.ControllerName {
		return fmt.Errorf("invalid gateway: unknown controller controller-name %q, "+
			"expecting %q", string(gc.Spec.ControllerName), config.ControllerName)
	}

	// this should already be validated but play it safe
	ref := gc.Spec.ParametersRef
	if ref == nil {
		return fmt.Errorf("empty ParametersRef in gateway-class spec: %#v", gc.Spec)
	}

	if ref.Group != gwapiv1a2.Group(stnrv1a1.GroupVersion.Group) {
		return fmt.Errorf("invalid Group in gateway-class spec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Name == "" {
		return fmt.Errorf("empty name in gateway-class spec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Namespace == nil || (ref.Namespace != nil && *ref.Namespace == "") {
		return fmt.Errorf("empty namespace in gateway-class spec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Kind != gwapiv1a2.Kind("GatewayConfig") {
		return fmt.Errorf("expecting ParametersRef to point to a gateway-config "+
			"resource: %#v", *gc.Spec.ParametersRef)
	}

	r.log.V(2).Info("validateGatewayClass", "gateway-class", store.GetObjectKey(gc), "result",
		"valid")

	return nil
}

func setGatewayClassStatusAccepted(gc *gwapiv1a2.GatewayClass, err error) {
	if err == nil {
		meta.SetStatusCondition(&gc.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.GatewayClassConditionStatusAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gc.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.GatewayClassReasonAccepted),
			Message: fmt.Sprintf("GatewayClass is now managed by controller %q",
				config.ControllerName),
		})
	} else {
		meta.SetStatusCondition(&gc.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1b1.GatewayClassConditionStatusAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gc.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1b1.GatewayClassReasonInvalidParameters),
			Message: fmt.Sprintf("controller %q failed to process GatewayClass: %s",
				config.ControllerName, err.Error()),
		})
	}
}

// helper for testing
func (r *Renderer) getGatewayClass() (*gwapiv1a2.GatewayClass, error) {
	gcs := store.GatewayClasses.GetAll()
	if len(gcs) == 0 {
		return nil, fmt.Errorf("no gateway-class found")
	}

	gc := gcs[0]
	if err := r.validateGatewayClass(gc); err != nil {
		return nil, err
	}

	return gc, nil
}
