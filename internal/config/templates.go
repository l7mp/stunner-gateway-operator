package config

import (
	// "fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apiutil "k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

var (
	StunnerdImage       = "l7mp/stunnerd:latest"
	TerminationGrace    = int64(3600)
	LivenessProbeAction = corev1.HTTPGetAction{
		Path:   "/live",
		Port:   apiutil.FromInt(8086),
		Scheme: "HTTP",
	}
	LivenessProbe = corev1.Probe{
		ProbeHandler:  corev1.ProbeHandler{HTTPGet: &LivenessProbeAction},
		PeriodSeconds: 15, SuccessThreshold: 1, FailureThreshold: 3, TimeoutSeconds: 1,
	}
	ReadinessProbeAction = corev1.HTTPGetAction{
		Path:   "/ready",
		Port:   apiutil.FromInt(8086),
		Scheme: "HTTP",
	}
	ReadinessProbe = corev1.Probe{
		ProbeHandler:  corev1.ProbeHandler{HTTPGet: &ReadinessProbeAction},
		PeriodSeconds: 15, SuccessThreshold: 1, FailureThreshold: 3, TimeoutSeconds: 1,
	}
	CPURequest      = resource.MustParse("500m")
	MemoryLimit     = resource.MustParse("50M")
	ResourceRequest = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: CPURequest,
	})
	ResourceLimit = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: MemoryLimit,
	})
)

func DataplaneTemplate(gateway client.Object) appv1.Deployment {
	selector := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			// Like `kubectl label ... -l "app=stunner"
			{
				Key:      opdefault.OwnedByLabelKey,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{opdefault.OwnedByLabelValue},
			},
			// Like `kubectl label ... -l  "stunner.l7mp.io/related-gateway-name=<gateway-name>"
			{
				Key:      opdefault.RelatedGatewayKey,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{gateway.GetName()},
			},
		},
	}

	optional := true
	configMapVolumeSource := corev1.ConfigMapVolumeSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: gateway.GetName(),
		},
		Optional: &optional,
	}

	podIPFieldSelector := corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"}
	podIPEnvVarSource := corev1.EnvVarSource{FieldRef: &podIPFieldSelector}

	volumeMounts := []corev1.VolumeMount{}
	if !EnvTestCompatibilityMode {
		// VolumeMounts are rejected by EnvTest
		volumeMounts = []corev1.VolumeMount{{
			Name:      "stunnerd-config-volume",
			MountPath: "/etc/stunnerd",
			ReadOnly:  true,
		}}
	}

	return appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gateway.GetName(),
			Namespace: gateway.GetNamespace(),
			Labels: map[string]string{
				opdefault.OwnedByLabelKey:   opdefault.OwnedByLabelValue,
				opdefault.RelatedGatewayKey: gateway.GetName(),
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
						opdefault.OwnedByLabelKey:   opdefault.OwnedByLabelValue,
						opdefault.RelatedGatewayKey: gateway.GetName(),
					},
					Annotations: map[string]string{
						opdefault.RelatedGatewayKey: types.NamespacedName{
							Namespace: gateway.GetNamespace(),
							Name:      gateway.GetName(),
						}.String(),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    opdefault.DefaultStunnerdInstanceName,
						Image:   StunnerdImage,
						Command: []string{"stunnerd"},
						Args:    []string{"-w", "-c", "/etc/stunnerd/stunnerd.conf", "--udp-thread-num=16"},
						Env: []corev1.EnvVar{{
							Name:      "STUNNER_ADDR",
							ValueFrom: &podIPEnvVarSource,
						}},
						Resources: corev1.ResourceRequirements{
							Limits:   ResourceLimit,
							Requests: ResourceRequest,
						},
						VolumeMounts:    volumeMounts,
						LivenessProbe:   &LivenessProbe,
						ReadinessProbe:  &ReadinessProbe,
						ImagePullPolicy: corev1.PullAlways,
					}},
					Volumes: []corev1.Volume{{
						Name: "stunnerd-config-volume",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &configMapVolumeSource,
						},
					}},
					TerminationGracePeriodSeconds: &TerminationGrace,
					HostNetwork:                   false,
				},
			},
		},
	}
}
