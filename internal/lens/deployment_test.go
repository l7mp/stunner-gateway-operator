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

func TestDeploymentEqualIgnoresUnownedOptionalFields(t *testing.T) {
	current := testDeployment()
	currentReplicas := int32(5)
	current.Spec.Replicas = &currentReplicas
	current.Spec.Template.Spec.TerminationGracePeriodSeconds = ptrInt64(30)
	current.Spec.Template.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}
	current.Spec.Template.Spec.Tolerations = []corev1.Toleration{{Key: "dedicated", Value: "edge"}}
	current.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
	current.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptrBool(true)}
	current.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "regcred"}}
	current.Spec.Template.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		MaxSkew:           1,
		TopologyKey:       "zone",
		WhenUnsatisfiable: corev1.ScheduleAnyway,
	}}

	desired := testDeployment()
	desired.Spec.Replicas = nil
	desired.Spec.Template.Spec.TerminationGracePeriodSeconds = nil
	desired.Spec.Template.Spec.Affinity = nil
	desired.Spec.Template.Spec.Tolerations = nil
	desired.Spec.Template.Spec.SecurityContext = nil
	desired.Spec.Template.Spec.Containers[0].SecurityContext = nil
	desired.Spec.Template.Spec.ImagePullSecrets = nil
	desired.Spec.Template.Spec.TopologySpreadConstraints = nil

	v := NewDeploymentLens(desired)
	assert.True(t, v.EqualResource(current),
		"unowned optional fields should be ignored in deployment equality")
}

func TestDeploymentApplyPreservesUnownedOptionalFields(t *testing.T) {
	current := testDeployment()
	currentReplicas := int32(5)
	current.Spec.Replicas = &currentReplicas
	current.Spec.Template.Spec.TerminationGracePeriodSeconds = ptrInt64(30)
	current.Spec.Template.Spec.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}}
	current.Spec.Template.Spec.Tolerations = []corev1.Toleration{{Key: "dedicated", Value: "edge"}}
	current.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
	current.Spec.Template.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptrBool(true)}
	current.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "regcred"}}
	current.Spec.Template.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		MaxSkew:           1,
		TopologyKey:       "zone",
		WhenUnsatisfiable: corev1.ScheduleAnyway,
	}}

	desired := testDeployment()
	desired.Spec.Replicas = nil
	desired.Spec.Template.Spec.TerminationGracePeriodSeconds = nil
	desired.Spec.Template.Spec.Affinity = nil
	desired.Spec.Template.Spec.Tolerations = nil
	desired.Spec.Template.Spec.SecurityContext = nil
	desired.Spec.Template.Spec.Containers[0].SecurityContext = nil
	desired.Spec.Template.Spec.ImagePullSecrets = nil
	desired.Spec.Template.Spec.TopologySpreadConstraints = nil

	v := NewDeploymentLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	require.NotNil(t, current.Spec.Replicas, "replicas should be preserved")
	assert.Equal(t, int32(5), *current.Spec.Replicas,
		"replicas should be preserved when not owned")
	require.NotNil(t, current.Spec.Template.Spec.TerminationGracePeriodSeconds,
		"termination grace should be preserved")
	assert.Equal(t, int64(30), *current.Spec.Template.Spec.TerminationGracePeriodSeconds,
		"termination grace should be preserved when not owned")
	assert.NotNil(t, current.Spec.Template.Spec.Affinity,
		"affinity should be preserved when not owned")
	assert.Len(t, current.Spec.Template.Spec.Tolerations, 1,
		"tolerations should be preserved when not owned")
	assert.NotNil(t, current.Spec.Template.Spec.SecurityContext,
		"security context should be preserved when not owned")
	require.NotNil(t, current.Spec.Template.Spec.Containers[0].SecurityContext,
		"container security context should be preserved when not owned")
	require.NotNil(t, current.Spec.Template.Spec.Containers[0].SecurityContext.Privileged,
		"container privileged setting should be preserved when not owned")
	assert.True(t, *current.Spec.Template.Spec.Containers[0].SecurityContext.Privileged,
		"container security context should be preserved when not owned")
	assert.Len(t, current.Spec.Template.Spec.ImagePullSecrets, 1,
		"image pull secrets should be preserved when not owned")
	assert.Len(t, current.Spec.Template.Spec.TopologySpreadConstraints, 1,
		"topology spread constraints should be preserved when not owned")
}

func TestDeploymentApplyCopiesOwnedEmptyOptionalSlices(t *testing.T) {
	current := testDeployment()
	current.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "regcred"}}
	current.Spec.Template.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
		MaxSkew:           1,
		TopologyKey:       "zone",
		WhenUnsatisfiable: corev1.ScheduleAnyway,
	}}

	desired := testDeployment()
	desired.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{}
	desired.Spec.Template.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{}

	v := NewDeploymentLens(desired)
	require.NoError(t, v.ApplyToResource(current), "apply failed")

	assert.Empty(t, current.Spec.Template.Spec.ImagePullSecrets,
		"owned empty image pull secrets should clear current values")
	assert.Empty(t, current.Spec.Template.Spec.TopologySpreadConstraints,
		"owned empty topology spread constraints should clear current values")
}

func ptrInt64(v int64) *int64 {
	return &v
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
