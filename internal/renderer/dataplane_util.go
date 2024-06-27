package renderer

import (
	"fmt"
	"net"
	"net/url"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

// createDeployment creates a new deployment for a managed Gateway. The later the label/annotation
// in the below orders the higher prio on conflict.
//
// Deployment-level labels:
//   - copy of labels from the related Gateway
//   - stunner.l7mp.io/owned-by=stunner
//   - stunner.l7mp.io/related-gateway-name=<gateway-name>
//   - stunner.l7mp.io/related-gateway-namespace=<gateway-namespace>
//
// (Name and namespace are different labels as '/' is not allowed in label values.)
//
// Deployment-level annotations:
//   - copy of annotations from the related gateway
//   - stunner.l7mp.io/related-gateway-name=<gateway-namespace/gateway-name>
//
// Pod-level labels:
//   - app=stunner
//   - stunner.l7mp.io/related-gateway-name=<gateway-name>
//   - stunner.l7mp.io/related-gateway-namespace=<gateway-namespace>
//
// These labels are used for the Deployment selector. Note that deployment-level annotations and
// labels are NOT propagated to the pods to avoid unexpected restarts.
func (r *Renderer) createDeployment(c *RenderContext) (*appv1.Deployment, error) {
	gw := c.gws.GetFirst()
	if gw == nil {
		r.log.Info("Internal error: trying to create Deployment with empty Gateway ref in managed mode")
		return nil, NewCriticalError(RenderingError)
	}

	dataplane, err := getDataplane(c)
	if err != nil {
		dataplaneName := opdefault.DefaultDataplaneName
		if c.gwConf != nil && c.gwConf.Spec.Dataplane != nil {
			dataplaneName = *c.gwConf.Spec.Dataplane
		}
		r.log.Error(err, "Cannot find Dataplane for Gateway",
			"gateway-config", store.GetObjectKey(c.gwConf),
			"dataplane", dataplaneName)

		return nil, err
	}

	// var deployment *appv1.Deployment
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

	// selectors
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
				Values:   []string{gw.GetName()},
			},
			// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-namespace=<gateway-namespace>"
			{
				Key:      opdefault.RelatedGatewayNamespace,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{gw.GetNamespace()},
			},
		},
	}

	deployment := appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gw.GetName(),
			Namespace: gw.GetNamespace(),
			Labels: map[string]string{
				opdefault.OwnedByLabelKey:         opdefault.OwnedByLabelValue,
				opdefault.RelatedGatewayKey:       gw.GetName(),
				opdefault.RelatedGatewayNamespace: gw.GetNamespace(),
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayKey: types.NamespacedName{
					Namespace: gw.GetNamespace(),
					Name:      gw.GetName(),
				}.String(),
			},
		},
		Spec: appv1.DeploymentSpec{
			Selector: &selector,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: store.MergeMetadata(dataplane.Spec.Labels,
						map[string]string{
							opdefault.AppLabelKey:             opdefault.AppLabelValue,
							opdefault.RelatedGatewayKey:       gw.GetName(),
							opdefault.RelatedGatewayNamespace: gw.GetNamespace(),
						}),
					Annotations: store.MergeMetadata(dataplane.Spec.Annotations,
						map[string]string{
							opdefault.RelatedGatewayKey: types.NamespacedName{
								Namespace: gw.GetNamespace(),
								Name:      gw.GetName(),
							}.String(),
						}),
				},
			},
		},
	}

	// copy deployment-level annotations and labels: overwrite whatever is set on the Gateway on conflict
	labs := store.MergeMetadata(gw.GetLabels(), deployment.GetLabels())
	deployment.SetLabels(labs)

	annotations := store.MergeMetadata(gw.GetAnnotations(), deployment.GetAnnotations())
	deployment.SetAnnotations(annotations)

	deployment.Spec.Template.Spec = corev1.PodSpec{
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
				Value: gw.GetName(),
			}, {
				Name:  "STUNNER_NAMESPACE", // gateway namespace for creating the stunnerd id
				Value: gw.GetNamespace(),
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

		if dataplane.Spec.ContainerSecurityContext != nil {
			c.SecurityContext = dataplane.Spec.ContainerSecurityContext.DeepCopy()
		}

		if dataplane.Spec.EnableMetricsEnpoint {
			c.Ports = []corev1.ContainerPort{{
				Name:          opdefault.DefaultMetricsPortName,
				ContainerPort: int32(stnrconfv1.DefaultMetricsPort),
				Protocol:      corev1.ProtocolTCP,
			}}
		}

		found = true
	}

	if !found {
		r.log.Info("Cannot find stunnerd container in dataplane Deployment template",
			"deployment", store.DumpObject(&deployment))
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
		podSpec.SecurityContext = dataplane.Spec.SecurityContext.DeepCopy()
	}

	// image pull secrets
	if len(dataplane.Spec.ImagePullSecrets) != 0 {
		podSpec.ImagePullSecrets = make([]corev1.LocalObjectReference, len(dataplane.Spec.ImagePullSecrets))
		for i := range dataplane.Spec.ImagePullSecrets {
			dataplane.Spec.ImagePullSecrets[i].DeepCopyInto(&podSpec.ImagePullSecrets[i])
		}
	}

	// topology spread constraints
	if len(dataplane.Spec.TopologySpreadConstraints) != 0 {
		podSpec.TopologySpreadConstraints = make([]corev1.TopologySpreadConstraint, len(dataplane.Spec.TopologySpreadConstraints))
		for i := range dataplane.Spec.TopologySpreadConstraints {
			dataplane.Spec.TopologySpreadConstraints[i].DeepCopyInto(&podSpec.TopologySpreadConstraints[i])
		}
	}

	// owned by the Gateway
	if err := controllerutil.SetOwnerReference(gw, &deployment, r.scheme); err != nil {
		r.log.Error(err, "Cannot set owner reference", "owner", store.GetObjectKey(gw),
			"reference", store.GetObjectKey(&deployment))
		return nil, NewCriticalError(RenderingError)
	}

	r.log.V(2).Info("Fnished creating Deployment",
		"gateway-class", store.GetObjectKey(c.gc),
		"gateway-config", store.GetObjectKey(c.gwConf),
		"gateway", store.GetObjectKey(gw),
		"dataplane", store.GetObjectKey(dataplane),
		"deployment", store.DumpObject(&deployment),
	)

	return &deployment, nil
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

// func defaultDeploymentSkeleton(gateway *gwapiv1.Gateway) appv1.Deployment {
// 	selector := metav1.LabelSelector{
// 		MatchExpressions: []metav1.LabelSelectorRequirement{
// 			// Like `kubectl label ... -l "app=stunner"
// 			{
// 				Key:      opdefault.AppLabelKey,
// 				Operator: metav1.LabelSelectorOpIn,
// 				Values:   []string{opdefault.AppLabelValue},
// 			},
// 			// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-name=<gateway-name>"
// 			{
// 				Key:      opdefault.RelatedGatewayKey,
// 				Operator: metav1.LabelSelectorOpIn,
// 				Values:   []string{gateway.GetName()},
// 			},
// 			// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-namespace=<gateway-namespace>"
// 			{
// 				Key:      opdefault.RelatedGatewayNamespace,
// 				Operator: metav1.LabelSelectorOpIn,
// 				Values:   []string{gateway.GetNamespace()},
// 			},
// 		},
// 	}

// 	dp := appv1.Deployment{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      gateway.GetName(),
// 			Namespace: gateway.GetNamespace(),
// 			Labels: map[string]string{
// 				opdefault.OwnedByLabelKey:         opdefault.OwnedByLabelValue,
// 				opdefault.RelatedGatewayKey:       gateway.GetName(),
// 				opdefault.RelatedGatewayNamespace: gateway.GetNamespace(),
// 			},
// 			Annotations: map[string]string{
// 				opdefault.RelatedGatewayKey: types.NamespacedName{
// 					Namespace: gateway.GetNamespace(),
// 					Name:      gateway.GetName(),
// 				}.String(),
// 			},
// 		},
// 		Spec: appv1.DeploymentSpec{
// 			Selector: &selector,
// 			Template: corev1.PodTemplateSpec{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Labels: map[string]string{
// 						opdefault.AppLabelKey:             opdefault.AppLabelValue,
// 						opdefault.RelatedGatewayKey:       gateway.GetName(),
// 						opdefault.RelatedGatewayNamespace: gateway.GetNamespace(),
// 					},
// 					Annotations: map[string]string{
// 						opdefault.RelatedGatewayKey: types.NamespacedName{
// 							Namespace: gateway.GetNamespace(),
// 							Name:      gateway.GetName(),
// 						}.String(),
// 					},
// 				},
// 			},
// 		},
// 	}

// 	// copy annotations and labels
// 	labs := labels.Merge(dp.GetLabels(), gateway.GetLabels())
// 	dp.SetLabels(labs)

// 	annotations := dp.GetAnnotations()
// 	if annotations == nil {
// 		annotations = make(map[string]string)
// 	}
// 	for k, v := range gateway.GetAnnotations() {
// 		annotations[k] = v
// 	}
// 	dp.SetAnnotations(annotations)

// 	return dp
// }

// // defaultDataplaneTemplate post-processes a deployment skeleton into a default dataplane
// func defaultDataplaneTemplate(c *RenderContext, gateway *gwapiv1.Gateway) *appv1.Deployment {
// 	podAddrFieldSelector := corev1.ObjectFieldSelector{FieldPath: "status.podIP"}
// 	podAddrEnvVarSource := corev1.EnvVarSource{FieldRef: &podAddrFieldSelector}
// 	livenessProbe, readinessProbe := getHealthCheckParameters(c)

// 	// CDS server address
// 	port := "13478"
// 	if addr, err := net.ResolveTCPAddr("tcp", config.ConfigDiscoveryAddress); err == nil {
// 		port = fmt.Sprintf("%d", addr.Port)
// 	}
// 	cdsAddr := url.URL{
// 		Scheme: "http",
// 		Host:   config.ConfigDiscoveryAddress,
// 	}
// 	if cdsAddr.Port() == "" {
// 		cdsAddr.Host = fmt.Sprintf("%s:%s", config.ConfigDiscoveryAddress, port)
// 	}

// 	dp := defaultDeploymentSkeleton(gateway)
// 	dp.Spec.Template.Spec = corev1.PodSpec{
// 		Containers: []corev1.Container{{
// 			Name:    opdefault.DefaultStunnerdInstanceName,
// 			Image:   config.StunnerdImage,
// 			Command: []string{"stunnerd"},
// 			// Enable config-discovery
// 			Args: []string{"-w", "--udp-thread-num=16"},
// 			// Disable config-discovery
// 			// Args:    []string{"-w", "-c", "/etc/stunnerd/stunnerd.conf", "--udp-thread-num=16"},
// 			Env: []corev1.EnvVar{{
// 				Name:      "STUNNER_ADDR", // default transport relay address
// 				ValueFrom: &podAddrEnvVarSource,
// 			}, {
// 				Name:  "STUNNER_NAME", // gateway name for creating the stunnerd id
// 				Value: gateway.GetName(),
// 			}, {
// 				Name:  "STUNNER_NAMESPACE", // gateway namespace for creating the stunnerd id
// 				Value: gateway.GetNamespace(),
// 			}, {
// 				Name:  "STUNNER_CONFIG_ORIGIN", // CDS server address
// 				Value: cdsAddr.String(),
// 			}},
// 			Resources: corev1.ResourceRequirements{
// 				Limits:   config.ResourceLimit,
// 				Requests: config.ResourceRequest,
// 			},
// 			LivenessProbe:   livenessProbe,
// 			ReadinessProbe:  readinessProbe,
// 			ImagePullPolicy: corev1.PullAlways,
// 		}},
// 		TerminationGracePeriodSeconds: &config.TerminationGrace,
// 		HostNetwork:                   false,
// 	}

// 	return &dp
// }

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
