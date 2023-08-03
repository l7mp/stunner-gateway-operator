package config

import (
	// "fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apiutil "k8s.io/apimachinery/pkg/util/intstr"
)

var (
	StunnerdImage       = "l7mp/stunnerd:latest"
	ConfigWatcherName   = "config-watcher"
	ConfigWatcherImage  = "kiwigrid/k8s-sidecar:latest"
	ConfigVolumeName    = "stunnerd-config-volume"
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
	CPURequest      = resource.MustParse("250m")
	MemoryLimit     = resource.MustParse("20M")
	ResourceRequest = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: CPURequest,
	})
	ResourceLimit = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: MemoryLimit,
	})
)
