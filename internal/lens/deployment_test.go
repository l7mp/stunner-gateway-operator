package lens

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.True(t, v.EqualResource(current),
		"expected deployment lenses to match after dropping defaulted fields")
}

func TestDeploymentEqualDetectsRealDiff(t *testing.T) {
	current := testDeployment()
	candidate := testDeployment()
	candidate.Spec.Template.Spec.Containers[0].Image = "stunner:v2"

	v := NewDeploymentLens(candidate)
	assert.False(t, v.EqualResource(current), "expected deployment image change to be detected")
}

func TestDeploymentEqualAfterDefaulting(t *testing.T) {
	desired := testDeployment()
	current := desired.DeepCopy()
	k8sscheme.Scheme.Default(current)

	v := NewDeploymentLens(desired)
	assert.True(t, v.EqualResource(current),
		"expected deployment no-op to be suppressed after scheme defaulting")
}

func TestDeploymentApply(t *testing.T) {
	current := testDeployment()
	desired := testDeployment()

	repl := int32(3)
	desired.Spec.Replicas = &repl
	desired.Spec.Template.Spec.Containers[0].Image = "stunner:v2"

	v := NewDeploymentLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.True(t, v.EqualResource(current), "expected applied deployment to match desired owned lens")
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
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Equal(t, "keep", current.Labels["external-label"],
		"deployment labels should retain external labels")
	assert.Equal(t, "set", current.Labels["owned-label"],
		"deployment labels should add owned labels")

	assert.Equal(t, "keep", current.Annotations["external-ann"],
		"deployment annotations should retain external annotations")
	assert.Equal(t, "set", current.Annotations["owned-ann"],
		"deployment annotations should add owned annotations")

	assert.NotContains(t, current.Spec.Template.Labels, "external-template-label",
		"pod template labels should be rewritten to owned labels")
	assert.Equal(t, "set", current.Spec.Template.Labels["owned-template-label"],
		"pod template labels should include owned labels")

	assert.NotContains(t, current.Spec.Template.Annotations, "external-template-ann",
		"pod template annotations should be rewritten to owned annotations")
	assert.Equal(t, "set", current.Spec.Template.Annotations["owned-template-ann"],
		"pod template annotations should include owned annotations")

	assert.Len(t, current.OwnerReferences, 2,
		"deployment ownerrefs should keep external and owned refs")
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
