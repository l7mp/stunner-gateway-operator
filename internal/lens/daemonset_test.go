package lens

import (
	"testing"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDaemonSetEqualDetectsRealDiff(t *testing.T) {
	current := testDaemonSet()
	candidate := testDaemonSet()
	candidate.Spec.Template.Spec.Containers[0].Image = "stunner:v2"

	v := NewDaemonSetLens(candidate)
	if v.EqualResource(current) {
		t.Fatalf("expected daemonset image change to be detected")
	}
}

func TestDaemonSetApplyPreservesExternalMetadata(t *testing.T) {
	current := testDaemonSet()
	current.Labels["external-label"] = "keep"
	current.Annotations["external-ann"] = "keep"
	current.Spec.Template.Labels["external-template-label"] = "keep"
	current.Spec.Template.Annotations["external-template-ann"] = "keep"
	current.OwnerReferences = append(current.OwnerReferences, metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "external-owner",
	})

	desired := testDaemonSet()
	desired.Labels["owned-label"] = "set"
	desired.Annotations["owned-ann"] = "set"
	desired.Spec.Template.Labels["owned-template-label"] = "set"
	desired.Spec.Template.Annotations["owned-template-ann"] = "set"

	v := NewDaemonSetLens(desired)
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if current.Labels["external-label"] != "keep" || current.Labels["owned-label"] != "set" {
		t.Fatalf("daemonset labels should retain external and add owned labels")
	}

	if current.Annotations["external-ann"] != "keep" || current.Annotations["owned-ann"] != "set" {
		t.Fatalf("daemonset annotations should retain external and add owned annotations")
	}

	if _, ok := current.Spec.Template.Labels["external-template-label"]; ok || current.Spec.Template.Labels["owned-template-label"] != "set" {
		t.Fatalf("pod template labels should be rewritten to owned labels")
	}

	if _, ok := current.Spec.Template.Annotations["external-template-ann"]; ok || current.Spec.Template.Annotations["owned-template-ann"] != "set" {
		t.Fatalf("pod template annotations should be rewritten to owned annotations")
	}
}

func testDaemonSet() *appv1.DaemonSet {
	return &appv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds",
			Namespace: "default",
			Labels:    map[string]string{"app": "stunner"},
			Annotations: map[string]string{
				"team": "edge",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1",
				Kind:       "Gateway",
				Name:       "gw",
			}},
		},
		Spec: appv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "stunner"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      map[string]string{"app": "stunner"},
					Annotations: map[string]string{"ann": "1"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "stunnerd",
						Image: "stunnerd:v1",
						Ports: []corev1.ContainerPort{{
							Name:          "metrics",
							ContainerPort: 8080,
							Protocol:      corev1.ProtocolTCP,
						}},
					}},
				},
			},
		},
	}
}
