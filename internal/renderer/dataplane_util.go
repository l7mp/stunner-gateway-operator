package renderer

import (
	// "fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// createDeployment creates a new deployment for a managed Gateway. Label selector rules are as follows:
// - deployment labeled with "app=stunner" and annotation "stunner.l7mp.io/related-gateway-name=<gateway-namespace/gateway-name>" is set
// - labels and annotations are merged verbatim on top of this from the corresponding Gateway object
// - all pods od the deployment are marked with label "app=stunner" AND "stunner.l7mp.io/related-gateway-name=<gateway-name>" (no namespace, as '/' is not allowed in label values)
// - deployment selector filters on these two labels
func (r *Renderer) createDeployment(c *RenderContext, name, namespace string) (*appv1.Deployment, error) {
	gw := c.gws.GetFirst()
	if gw == nil {
		r.log.Info("ERROR: createDeployment called with empty Gateway ref in managed mode")
		return nil, NewCriticalError(RenderingError)
	}

	// find the Dataplane CR for this Gateway
	dataplaneName := opdefault.DefaultDataplaneName
	if c.gwConf.Spec.Dataplane != nil {
		dataplaneName = *c.gwConf.Spec.Dataplane
	}
	dataplane := store.Dataplanes.GetObject(types.NamespacedName{Name: dataplaneName})
	if dataplane == nil {
		err := NewCriticalError(InvalidDataplane)
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
				opdefault.RelatedGatewayKey: store.GetObjectKey(gw),
			},
		},
		Spec: appv1.DeploymentSpec{},
	}

	// copy annotations and labels
	labs := labels.Merge(deployment.GetLabels(), gw.GetLabels())
	deployment.SetLabels(labs)

	annotations := deployment.GetAnnotations()
	for k, v := range gw.GetAnnotations() {
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
	// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-name=<gateway-name>"
	relatedReq := metav1.LabelSelectorRequirement{
		Key:      opdefault.RelatedGatewayKey,
		Operator: metav1.LabelSelectorOpIn,
		Values:   []string{gw.GetName()},
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
				opdefault.OwnedByLabelKey:   opdefault.OwnedByLabelValue,
				opdefault.RelatedGatewayKey: gw.GetName(),
			},
		},
	}

	podSpec := &deployment.Spec.Template.Spec

	// containers verbatim
	podSpec.Containers = make([]corev1.Container, len(dataplane.Spec.Containers))
	for i := range dataplane.Spec.Containers {
		dataplane.Spec.Containers[i].DeepCopyIntoCoreV1(&podSpec.Containers[i])
	}

	// volumes
	podSpec.Volumes = make([]corev1.Volume, len(dataplane.Spec.Volumes))
	for i := range dataplane.Spec.Volumes {
		podSpec.Volumes[i].DeepCopyInto(&dataplane.Spec.Volumes[i])
	}

	// grace
	if dataplane.Spec.TerminationGracePeriodSeconds != nil {
		podSpec.TerminationGracePeriodSeconds = dataplane.Spec.TerminationGracePeriodSeconds
	}

	// hostnetwork
	podSpec.HostNetwork = dataplane.Spec.HostNetwork

	// affinity
	if dataplane.Spec.Affinity != nil {
		podSpec.Affinity = dataplane.Spec.Affinity
	}

	// owned by the Gateway
	if err := controllerutil.SetOwnerReference(gw, deployment, r.scheme); err != nil {
		r.log.Error(err, "cannot set owner reference", "owner", store.GetObjectKey(gw),
			"reference", store.GetObjectKey(deployment))
		return nil, NewCriticalError(RenderingError)
	}

	// fmt.Printf("%#v\n", deployment)

	return deployment, nil
}
