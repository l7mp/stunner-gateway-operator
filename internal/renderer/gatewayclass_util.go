package renderer

import (
	"fmt"

	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *renderer) getGatewayClasses() []*gwapiv1.GatewayClass {
	ret := []*gwapiv1.GatewayClass{}

	for _, gc := range store.GatewayClasses.GetAll() {
		if err := r.validateGatewayClass(gc); err != nil {
			r.log.Error(err, "Invalid gateway-class", "gateway-class", store.GetObjectKey(gc))
			continue
		}

		ret = append(ret, gc)
	}

	r.log.V(2).Info("Finished searching for gateway-classes", "found", fmt.Sprintf("%d gateway-classes", len(ret)))

	return ret
}

func (r *renderer) validateGatewayClass(gc *gwapiv1.GatewayClass) error {
	// play it safe
	if string(gc.Spec.ControllerName) != config.ControllerName {
		return fmt.Errorf("Invalid Gateway: unknown controller controller-name %q, "+
			"expecting %q", string(gc.Spec.ControllerName), config.ControllerName)
	}

	// this should already be validated but play it safe
	ref := gc.Spec.ParametersRef
	if ref == nil {
		return fmt.Errorf("Empty ParametersRef in gateway-class spec: %#v", gc.Spec)
	}

	if ref.Group != gwapiv1.Group(stnrgwv1.GroupVersion.Group) {
		return fmt.Errorf("Invalid Group in gateway-class spec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Name == "" {
		return fmt.Errorf("Empty name in gateway-class spec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Namespace == nil || (ref.Namespace != nil && *ref.Namespace == "") {
		return fmt.Errorf("Empty namespace in gateway-class spec: %#v",
			*gc.Spec.ParametersRef)
	}

	if ref.Kind != gwapiv1.Kind("GatewayConfig") {
		return fmt.Errorf("Expecting ParametersRef to point to a gateway-config "+
			"resource: %#v", *gc.Spec.ParametersRef)
	}

	r.log.V(4).Info("Finished validating gateway-class", "gateway-class",
		store.GetObjectKey(gc), "result", "valid")

	return nil
}

func setGatewayClassStatusAccepted(gc *gwapiv1.GatewayClass, err error) {
	if err == nil {
		meta.SetStatusCondition(&gc.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1.GatewayClassConditionStatusAccepted),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: gc.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.GatewayClassReasonAccepted),
			Message: fmt.Sprintf("GatewayClass is now managed by controller %q",
				config.ControllerName),
		})
	} else {
		meta.SetStatusCondition(&gc.Status.Conditions, metav1.Condition{
			Type:               string(gwapiv1.GatewayClassConditionStatusAccepted),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: gc.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             string(gwapiv1.GatewayClassReasonInvalidParameters),
			Message: fmt.Sprintf("controller %q failed to process GatewayClass: %s",
				config.ControllerName, err.Error()),
		})
	}
}

// helper for testing
//
//nolint:unused
func (r *renderer) getGatewayClass() (*gwapiv1.GatewayClass, error) {
	gcs := store.GatewayClasses.GetAll()
	if len(gcs) == 0 {
		return nil, fmt.Errorf("No gateway-class found")
	}

	gc := gcs[0]
	if err := r.validateGatewayClass(gc); err != nil {
		return nil, err
	}

	return gc, nil
}
