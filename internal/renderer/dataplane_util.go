package renderer

import (
	// "fmt"

	appv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// createDeployment creates a new deployment for a managed Gateway. Label selector rules are as follows:
// - deployment labeled with "app=stunner" and annotation "stunner.l7mp.io/related-gateway-name=<gateway-namespace/gateway-name>" is set
// - labels and annotations are merged verbatim on top of this from the corresponding Gateway object
// - all pods od the deployment are marked with label "app=stunner" AND "stunner.l7mp.io/related-gateway-name=<gateway-name>" (no namespace, as '/' is not allowed in label values)
// - deployment selector filters on these two labels
func (r *Renderer) createDeployment(c *RenderContext) (*appv1.Deployment, error) {
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

	deployment := config.DataplaneTemplate(gw)

	// copy annotations and labels
	labs := labels.Merge(deployment.GetLabels(), gw.GetLabels())
	deployment.SetLabels(labs)

	annotations := deployment.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	for k, v := range gw.GetAnnotations() {
		annotations[k] = v
	}
	deployment.SetAnnotations(annotations)

	// copy spec
	if dataplane.Spec.Replicas != nil {
		deployment.Spec.Replicas = dataplane.Spec.Replicas
	}

	// grace
	podSpec := &deployment.Spec.Template.Spec
	if dataplane.Spec.TerminationGracePeriodSeconds != nil {
		podSpec.TerminationGracePeriodSeconds = dataplane.Spec.TerminationGracePeriodSeconds
	}

	// resource
	found := false
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == opdefault.DefaultStunnerdInstanceName {
			found = true
			c := &podSpec.Containers[i]
			if dataplane.Spec.Image != "" {
				c.Image = dataplane.Spec.Image
			}
			if dataplane.Spec.ImagePullPolicy != nil {
				c.ImagePullPolicy = *dataplane.Spec.ImagePullPolicy
			}
			if len(dataplane.Spec.Command) != 0 {
				c.Command = make([]string, len(dataplane.Spec.Command))
				copy(c.Command, dataplane.Spec.Command)
			}
			if len(dataplane.Spec.Args) != 0 {
				c.Args = make([]string, len(dataplane.Spec.Args))
				copy(c.Args, dataplane.Spec.Args)
			}
			if dataplane.Spec.Resources != nil {
				dataplane.Spec.Resources.DeepCopyInto(&c.Resources)
			}
		}
	}

	if !found {
		r.log.Info("cannot find stunnerd container in dataplane Deployment template",
			"deployment", store.DumpObject(&deployment))
		return nil, NewCriticalError(RenderingError)
	}

	// hostnetwork
	podSpec.HostNetwork = dataplane.Spec.HostNetwork

	// affinity
	if dataplane.Spec.Affinity != nil {
		podSpec.Affinity = dataplane.Spec.Affinity
	}

	// owned by the Gateway
	if err := controllerutil.SetOwnerReference(gw, &deployment, r.scheme); err != nil {
		r.log.Error(err, "cannot set owner reference", "owner", store.GetObjectKey(gw),
			"reference", store.GetObjectKey(&deployment))
		return nil, NewCriticalError(RenderingError)
	}

	r.log.V(2).Info("createDeployment: ready",
		"gateway-class", store.GetObjectKey(c.gc),
		"gateway-config", store.GetObjectKey(c.gwConf),
		"gateway", store.GetObjectKey(gw),
		"dataplane", store.GetObjectKey(dataplane),
		"deployment", store.DumpObject(&deployment),
	)

	return &deployment, nil
}
