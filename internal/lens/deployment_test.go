package lens

import (
	"testing"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
)

func TestDeploymentEqualIgnoresDefaultedFields(t *testing.T) {
	current := testDeployment()
	candidate := testDeployment()

	current.Spec.Template.Spec.Containers[0].Ports[0].HostPort = 8080
	current.Spec.Template.Spec.Containers[0].Env[0].ValueFrom.FieldRef.APIVersion = "v1"
	current.Spec.Template.Spec.Containers[0].TerminationMessagePath = "/dev/termination-log"
	current.Spec.Template.Spec.Containers[0].TerminationMessagePolicy = corev1.TerminationMessageReadFile

	v := NewDeploymentLens(candidate)
	if !v.EqualResource(current) {
		t.Fatalf("expected deployment lenses to match after dropping defaulted fields")
	}
}

func TestDeploymentEqualDetectsRealDiff(t *testing.T) {
	current := testDeployment()
	candidate := testDeployment()
	candidate.Spec.Template.Spec.Containers[0].Image = "stunner:v2"

	v := NewDeploymentLens(candidate)
	if v.EqualResource(current) {
		t.Fatalf("expected deployment image change to be detected")
	}
}

func TestDeploymentEqualAfterDefaulting(t *testing.T) {
	desired := testDeployment()
	current := desired.DeepCopy()
	k8sscheme.Scheme.Default(current)

	v := NewDeploymentLens(desired)
	if !v.EqualResource(current) {
		t.Fatalf("expected deployment no-op to be suppressed after scheme defaulting")
	}
}

func TestDeploymentApply(t *testing.T) {
	current := testDeployment()
	desired := testDeployment()

	repl := int32(3)
	desired.Spec.Replicas = &repl
	desired.Spec.Template.Spec.Containers[0].Image = "stunner:v2"

	v := NewDeploymentLens(desired)
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !v.EqualResource(current) {
		t.Fatalf("expected applied deployment to match desired owned lens")
	}
}

func TestDeploymentApplyPreservesExternalMetadata(t *testing.T) {
	current := testDeployment()
	current.Labels["external-label"] = "keep"
	current.Annotations["external-ann"] = "keep"
	current.Spec.Template.Labels["external-template-label"] = "keep"
	current.Spec.Template.Annotations["external-template-ann"] = "keep"
	current.OwnerReferences = append(current.OwnerReferences, metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "external-owner",
	})

	desired := testDeployment()
	desired.Labels["owned-label"] = "set"
	desired.Annotations["owned-ann"] = "set"
	desired.Spec.Template.Labels["owned-template-label"] = "set"
	desired.Spec.Template.Annotations["owned-template-ann"] = "set"

	v := NewDeploymentLens(desired)
	if err := v.ApplyToResource(current); err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if current.Labels["external-label"] != "keep" || current.Labels["owned-label"] != "set" {
		t.Fatalf("deployment labels should retain external and add owned labels")
	}

	if current.Annotations["external-ann"] != "keep" || current.Annotations["owned-ann"] != "set" {
		t.Fatalf("deployment annotations should retain external and add owned annotations")
	}

	if _, ok := current.Spec.Template.Labels["external-template-label"]; ok || current.Spec.Template.Labels["owned-template-label"] != "set" {
		t.Fatalf("pod template labels should be rewritten to owned labels")
	}

	if _, ok := current.Spec.Template.Annotations["external-template-ann"]; ok || current.Spec.Template.Annotations["owned-template-ann"] != "set" {
		t.Fatalf("pod template annotations should be rewritten to owned annotations")
	}

	if len(current.OwnerReferences) != 2 {
		t.Fatalf("deployment ownerrefs should keep external and owned refs")
	}
}

func testDeployment() *appv1.Deployment {
	replicas := int32(2)
	return &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp",
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
		Spec: appv1.DeploymentSpec{
			Replicas: &replicas,
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
						Env: []corev1.EnvVar{{
							Name: "POD_IP",
							ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "status.podIP",
							}},
						}},
					}},
				},
			},
		},
	}
}
