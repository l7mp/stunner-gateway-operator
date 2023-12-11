package renderer

import (
	"fmt"
	"net"
	"net/url"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
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

	dataplane, err := getDataplane(c)
	if err != nil {
		dataplaneName := opdefault.DefaultDataplaneName
		if c.gwConf != nil && c.gwConf.Spec.Dataplane != nil {
			dataplaneName = *c.gwConf.Spec.Dataplane
		}
		r.log.Error(err, "cannot find Dataplane for Gateway",
			"gateway-config", store.GetObjectKey(c.gwConf),
			"dataplane", dataplaneName)

		return nil, err
	}

	// var deployment *appv1.Deployment

	// switch dataplane.Spec.Template {
	// case config.ConfigWatcherName:
	// 	// dataplane with config-watcher sidecar
	// 	deployment = configWatcherDataplaneTemplate(gw)
	// default:
	// 	// standard dataplane: single stunnerd pod with included config file watcher
	// 	deployment = defaultDataplaneTemplate(gw)
	// }

	deployment := defaultDataplaneTemplate(c, gw)

	// post process

	// copy spec
	if dataplane.Spec.Replicas != nil {
		deployment.Spec.Replicas = dataplane.Spec.Replicas
	}

	// grace
	podSpec := &deployment.Spec.Template.Spec
	if dataplane.Spec.TerminationGracePeriodSeconds != nil {
		podSpec.TerminationGracePeriodSeconds = dataplane.Spec.TerminationGracePeriodSeconds
	}

	found := false
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name != opdefault.DefaultStunnerdInstanceName {
			continue
		}

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
		if len(dataplane.Spec.Env) != 0 {
			// append
			if c.Env == nil {
				c.Env = []corev1.EnvVar{}
			}
			c.Env = append(c.Env, dataplane.Spec.Env...)
		}
		if dataplane.Spec.Resources != nil {
			dataplane.Spec.Resources.DeepCopyInto(&c.Resources)
		}

		found = true
	}

	if !found {
		r.log.Info("cannot find stunnerd container in dataplane Deployment template",
			"deployment", store.DumpObject(deployment))
		return nil, NewCriticalError(RenderingError)
	}

	// hostnetwork
	podSpec.HostNetwork = dataplane.Spec.HostNetwork

	// affinity
	if dataplane.Spec.Affinity != nil {
		podSpec.Affinity = dataplane.Spec.Affinity
	}

	// tolerations
	if dataplane.Spec.Tolerations != nil {
		podSpec.Tolerations = dataplane.Spec.Tolerations
	}

	// security context
	if dataplane.Spec.SecurityContext != nil {
		podSpec.SecurityContext = dataplane.Spec.SecurityContext
	}

	// owned by the Gateway
	if err := controllerutil.SetOwnerReference(gw, deployment, r.scheme); err != nil {
		r.log.Error(err, "cannot set owner reference", "owner", store.GetObjectKey(gw),
			"reference", store.GetObjectKey(deployment))
		return nil, NewCriticalError(RenderingError)
	}

	r.log.V(2).Info("createDeployment: ready",
		"gateway-class", store.GetObjectKey(c.gc),
		"gateway-config", store.GetObjectKey(c.gwConf),
		"gateway", store.GetObjectKey(gw),
		"dataplane", store.GetObjectKey(dataplane),
		"deployment", store.DumpObject(deployment),
	)

	return deployment, nil
}

func getDataplane(c *RenderContext) (*stnrgwv1.Dataplane, error) {
	dataplaneName := opdefault.DefaultDataplaneName
	if c.gwConf != nil && c.gwConf.Spec.Dataplane != nil {
		dataplaneName = *c.gwConf.Spec.Dataplane
	}
	dataplane := store.Dataplanes.GetObject(types.NamespacedName{Name: dataplaneName})
	if dataplane == nil {
		err := NewCriticalError(InvalidDataplane)
		return nil, err
	}

	return dataplane, nil
}

func getHealthCheckParameters(c *RenderContext) (*corev1.Probe, *corev1.Probe) {
	if c.dp != nil && c.dp.Spec.DisableHealthCheck {
		return nil, nil
	}

	livenessProbeAction := config.LivenessProbeAction.DeepCopy()
	livenessProbe := config.LivenessProbe.DeepCopy()
	livenessProbe.ProbeHandler.HTTPGet = livenessProbeAction

	readinessProbeAction := config.ReadinessProbeAction.DeepCopy()
	readinessProbe := config.ReadinessProbe.DeepCopy()
	readinessProbe.ProbeHandler.HTTPGet = readinessProbeAction

	return livenessProbe, readinessProbe
}

// ///////
// TEMPLATES
// //////

func defaultDeploymentSkeleton(gateway *gwapiv1.Gateway) appv1.Deployment {
	selector := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			// Like `kubectl label ... -l "app=stunner"
			{
				Key:      opdefault.AppLabelKey,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{opdefault.AppLabelValue},
			},
			// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			{
				Key:      opdefault.RelatedGatewayKey,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{gateway.GetName()},
			},
			// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-namespace=<gateway-namespace>"
			{
				Key:      opdefault.RelatedGatewayNamespace,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{gateway.GetNamespace()},
			},
		},
	}

	dp := appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gateway.GetName(),
			Namespace: gateway.GetNamespace(),
			Labels: map[string]string{
				opdefault.OwnedByLabelKey:         opdefault.OwnedByLabelValue,
				opdefault.RelatedGatewayKey:       gateway.GetName(),
				opdefault.RelatedGatewayNamespace: gateway.GetNamespace(),
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayKey: types.NamespacedName{
					Namespace: gateway.GetNamespace(),
					Name:      gateway.GetName(),
				}.String(),
			},
		},
		Spec: appv1.DeploymentSpec{
			Selector: &selector,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						opdefault.AppLabelKey:             opdefault.AppLabelValue,
						opdefault.RelatedGatewayKey:       gateway.GetName(),
						opdefault.RelatedGatewayNamespace: gateway.GetNamespace(),
					},
					Annotations: map[string]string{
						opdefault.RelatedGatewayKey: types.NamespacedName{
							Namespace: gateway.GetNamespace(),
							Name:      gateway.GetName(),
						}.String(),
					},
				},
			},
		},
	}

	// copy annotations and labels
	labs := labels.Merge(dp.GetLabels(), gateway.GetLabels())
	dp.SetLabels(labs)

	annotations := dp.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	for k, v := range gateway.GetAnnotations() {
		annotations[k] = v
	}
	dp.SetAnnotations(annotations)

	return dp
}

// defaultDataplaneTemplate post-processes a deployment skeleton into a default dataplane
func defaultDataplaneTemplate(c *RenderContext, gateway *gwapiv1.Gateway) *appv1.Deployment {
	podAddrFieldSelector := corev1.ObjectFieldSelector{FieldPath: "status.podIP"}
	podAddrEnvVarSource := corev1.EnvVarSource{FieldRef: &podAddrFieldSelector}
	livenessProbe, readinessProbe := getHealthCheckParameters(c)

	// CDS server address
	port := "13478"
	if addr, err := net.ResolveTCPAddr("tcp", config.ConfigDiscoveryAddress); err == nil {
		port = fmt.Sprintf("%d", addr.Port)
	}
	cdsAddr := url.URL{
		Scheme: "http",
		Host:   config.ConfigDiscoveryAddress,
	}
	if cdsAddr.Port() == "" {
		cdsAddr.Host = fmt.Sprintf("%s:%s", config.ConfigDiscoveryAddress, port)
	}

	dp := defaultDeploymentSkeleton(gateway)
	dp.Spec.Template.Spec = corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:    opdefault.DefaultStunnerdInstanceName,
			Image:   config.StunnerdImage,
			Command: []string{"stunnerd"},
			// Enable config-discovery
			Args: []string{"-w", "--udp-thread-num=16"},
			// Disable config-discovery
			// Args:    []string{"-w", "-c", "/etc/stunnerd/stunnerd.conf", "--udp-thread-num=16"},
			Env: []corev1.EnvVar{{
				Name:      "STUNNER_ADDR", // default transport relay address
				ValueFrom: &podAddrEnvVarSource,
			}, {
				Name:  "STUNNER_NAME", // gateway name for creating the stunnerd id
				Value: gateway.GetName(),
			}, {
				Name:  "STUNNER_NAMESPACE", // gateway namespace for creating the stunnerd id
				Value: gateway.GetNamespace(),
			}, {
				Name:  "STUNNER_CONFIG_ORIGIN", // CDS server address
				Value: cdsAddr.String(),
			}},
			Resources: corev1.ResourceRequirements{
				Limits:   config.ResourceLimit,
				Requests: config.ResourceRequest,
			},
			LivenessProbe:   livenessProbe,
			ReadinessProbe:  readinessProbe,
			ImagePullPolicy: corev1.PullAlways,
		}},
		TerminationGracePeriodSeconds: &config.TerminationGrace,
		HostNetwork:                   false,
	}

	return &dp
}

// // configWatcherDataplaneTemplate post-processes a deployment skeleton into a dataplane with a config-watcher sidecar.
// func configWatcherDataplaneTemplate(gateway *gwapiv1.Gateway) *appv1.Deployment {
// 	podIPFieldSelector := corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"}
// 	podIPEnvVarSource := corev1.EnvVarSource{FieldRef: &podIPFieldSelector}

// 	emptyDir := corev1.EmptyDirVolumeSource{}
// 	volumeMounts := []corev1.VolumeMount{{
// 		Name:      config.ConfigVolumeName,
// 		MountPath: "/etc/stunnerd",
// 		ReadOnly:  true,
// 	}}

// 	dp := defaultDeploymentSkeleton(gateway)
// 	dp.Spec.Template.Spec = corev1.PodSpec{
// 		Containers: []corev1.Container{{
// 			Name:    opdefault.DefaultStunnerdInstanceName,
// 			Image:   config.StunnerdImage,
// 			Command: []string{"stunnerd"},
// 			Args:    []string{"-w", "-c", "/etc/stunnerd/stunnerd.conf", "--udp-thread-num=16"},
// 			Env: []corev1.EnvVar{{
// 				Name:      "STUNNER_ADDR",
// 				ValueFrom: &podIPEnvVarSource,
// 			}},
// 			Resources: corev1.ResourceRequirements{
// 				Limits:   config.ResourceLimit,
// 				Requests: config.ResourceRequest,
// 			},
// 			VolumeMounts:    volumeMounts,
// 			LivenessProbe:   &config.LivenessProbe,
// 			ReadinessProbe:  &config.ReadinessProbe,
// 			ImagePullPolicy: corev1.PullAlways,
// 		}, {
// 			Name:  "config-watcher",
// 			Image: config.ConfigWatcherImage,
// 			Env: []corev1.EnvVar{{
// 				Name:  "LABEL",
// 				Value: "stunner.l7mp.io/owned-by",
// 			}, {
// 				Name:  "LABEL_VALUE",
// 				Value: "stunner",
// 			}, {
// 				Name:  "FOLDER",
// 				Value: "/etc/stunnerd",
// 			}, {
// 				Name:  "RESOURCE",
// 				Value: "configmap",
// 			}, {
// 				Name:  "NAMESPACE",
// 				Value: "stunner",
// 			}},
// 			Resources: corev1.ResourceRequirements{
// 				Limits:   config.ResourceLimit,
// 				Requests: config.ResourceRequest,
// 			},
// 			VolumeMounts:    volumeMounts,
// 			LivenessProbe:   &config.LivenessProbe,
// 			ReadinessProbe:  &config.ReadinessProbe,
// 			ImagePullPolicy: corev1.PullIfNotPresent,
// 		}},
// 		Volumes: []corev1.Volume{{
// 			Name:         config.ConfigVolumeName,
// 			VolumeSource: corev1.VolumeSource{EmptyDir: &emptyDir},
// 		}},
// 		TerminationGracePeriodSeconds: &config.TerminationGrace,
// 		HostNetwork:                   false,
// 	}

// 	return &dp
// }
