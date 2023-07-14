package renderer

import (
	// "fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// createDeployment creates a new deployment for a managed Gateway. Label selector rules are as follows:
// - labels and annotations are copied verbatim from the corresponding Dataplane object
// - all pods for the gateway marked with labels "app=stunner" and ""stunner.l7mp.io/related-gateway-name=<gateway namespace/name>"
// - deployment selector filters on these two labels
func (r *Renderer) createDeployment(c *RenderContext, name, namespace string) (*appv1.Deployment, error) {
	gw := c.gws.GetFirst()

	// find the Dataplane CR for this Gateway
	dataplaneName := opdefault.DefaultDataplaneName
	if c.gwConf.Spec.Dataplane != nil {
		dataplaneName = *c.gwConf.Spec.Dataplane
	}

	dataplane := store.Dataplanes.GetObject(types.NamespacedName{Name: dataplaneName})
	if dataplane == nil {
		err := NewCriticalError(RenderingError)
		r.log.Error(err, "cannot find Dataplane for Gateway",
			"gateway-config", store.GetObjectKey(c.gwConf),
			"gateway", store.GetObjectKey(gw),
			"dataplane", dataplaneName,
		)
		return nil, err
	}

	deployment := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayAnnotationKey: store.GetObjectKey(gw),
			},
		},
		Spec: appv1.DeploymentSpec{},
	}

	// copy annotations and labels
	labs := deployment.GetLabels()
	for k, v := range dataplane.GetLabels() {
		labs[k] = v
	}
	deployment.SetLabels(labs)

	annotations := deployment.GetAnnotations()
	for k, v := range dataplane.GetAnnotations() {
		annotations[k] = v
	}
	deployment.SetAnnotations(annotations)

	// prepare label selector
	// Like `kubectl label ... -l "app=stunner"
	appReq := metav1.LabelSelectorRequirement{
		Key:      opdefault.OwnedByLabelKey,
		Operator: metav1.LabelSelectorOpIn,
		Values:   []string{opdefault.OwnedByLabelValue},
	}
	// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-name=<gateway namespace/name>"
	relatedReq := metav1.LabelSelectorRequirement{
		Key:      opdefault.RelatedGatewayAnnotationKey,
		Operator: metav1.LabelSelectorOpIn,
		Values:   []string{store.GetObjectKey(gw)},
	}

	deployment.Spec.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{appReq, relatedReq},
	}

	// copy spec
	if dataplane.Spec.Replicas != nil {
		deployment.Spec.Replicas = dataplane.Spec.Replicas
	}

	if dataplane.Spec.Strategy != nil {
		deployment.Spec.Strategy = *dataplane.Spec.Strategy
	}

	// copy pod template spec
	deployment.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayAnnotationKey: store.GetObjectKey(gw),
			},
		},
	}

	podspec := &deployment.Spec.Template.Spec

	// containers verbatim
	podspec.Containers = make([]corev1.Container, len(dataplane.Spec.Containers))
	for i := range dataplane.Spec.Containers {
		dataplane.Spec.Containers[i].DeepCopyIntoCoreV1(&podspec.Containers[i])
	}

	// grace
	if dataplane.Spec.TerminationGracePeriodSeconds != nil {
		podspec.TerminationGracePeriodSeconds = dataplane.Spec.TerminationGracePeriodSeconds
	}

	// hostnetwork
	podspec.HostNetwork = dataplane.Spec.HostNetwork

	// affinity
	if dataplane.Spec.Affinity != nil {
		podspec.Affinity = dataplane.Spec.Affinity
	}

	// owned by the Gateway
	if err := controllerutil.SetOwnerReference(gw, deployment, r.scheme); err != nil {
		r.log.Error(err, "cannot set owner reference", "owner", store.GetObjectKey(gw),
			"reference", store.GetObjectKey(deployment))
		return nil, NewCriticalError(RenderingError)
	}

	return deployment, nil
}
