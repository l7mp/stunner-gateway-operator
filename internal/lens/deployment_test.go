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

func TestDeploymentUnownedOptionalFields(t *testing.T) {
	for _, tc := range unownedPodSpecCases() {
		t.Run(tc.name+"/Equal", func(t *testing.T) {
			current := deploymentWithNoOwnedOptionals()
			tc.mutate(&current.Spec.Template.Spec)
			desired := deploymentWithNoOwnedOptionals()

			v := NewDeploymentLens(desired)
			assert.True(t, v.EqualResource(current),
				"unowned %s should be ignored in deployment equality", tc.name)
		})

		t.Run(tc.name+"/Apply", func(t *testing.T) {
			current := deploymentWithNoOwnedOptionals()
			tc.mutate(&current.Spec.Template.Spec)
			desired := deploymentWithNoOwnedOptionals()

			v := NewDeploymentLens(desired)
			require.NoError(t, v.ApplyToResource(current), "apply failed")
			tc.check(t, &current.Spec.Template.Spec)
		})
	}

	// Replicas lives on DeploymentSpec, so it is exercised here only.
	t.Run("Replicas/Equal", func(t *testing.T) {
		current := deploymentWithNoOwnedOptionals()
		r := int32(5)
		current.Spec.Replicas = &r
		desired := deploymentWithNoOwnedOptionals()

		v := NewDeploymentLens(desired)
		assert.True(t, v.EqualResource(current),
			"unowned Replicas should be ignored in deployment equality")
	})
	t.Run("Replicas/Apply", func(t *testing.T) {
		current := deploymentWithNoOwnedOptionals()
		r := int32(5)
		current.Spec.Replicas = &r
		desired := deploymentWithNoOwnedOptionals()

		v := NewDeploymentLens(desired)
		require.NoError(t, v.ApplyToResource(current), "apply failed")
		require.NotNil(t, current.Spec.Replicas, "replicas should be preserved")
		assert.Equal(t, int32(5), *current.Spec.Replicas,
			"replicas should be preserved when not owned")
	})
}

func deploymentWithNoOwnedOptionals() *appv1.Deployment {
	d := testDeployment()
	d.Spec.Replicas = nil
	clearOwnedPodSpecOptionals(&d.Spec.Template.Spec)
	return d
}

type podSpecFieldCase struct {
	name   string
	mutate func(s *corev1.PodSpec)
	check  func(t *testing.T, s *corev1.PodSpec)
}

func unownedPodSpecCases() []podSpecFieldCase {
	return []podSpecFieldCase{
		{
			name:   "TerminationGracePeriodSeconds",
			mutate: func(s *corev1.PodSpec) { s.TerminationGracePeriodSeconds = ptrInt64(30) },
			check: func(t *testing.T, s *corev1.PodSpec) {
				require.NotNil(t, s.TerminationGracePeriodSeconds,
					"termination grace should be preserved")
				assert.Equal(t, int64(30), *s.TerminationGracePeriodSeconds,
					"termination grace should be preserved when not owned")
			},
		},
		{
			name:   "Affinity",
			mutate: func(s *corev1.PodSpec) { s.Affinity = &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}} },
			check: func(t *testing.T, s *corev1.PodSpec) {
				assert.NotNil(t, s.Affinity, "affinity should be preserved when not owned")
			},
		},
		{
			name: "Tolerations",
			mutate: func(s *corev1.PodSpec) {
				s.Tolerations = []corev1.Toleration{{Key: "dedicated", Value: "edge"}}
			},
			check: func(t *testing.T, s *corev1.PodSpec) {
				assert.Len(t, s.Tolerations, 1, "tolerations should be preserved when not owned")
			},
		},
		{
			name:   "SecurityContext",
			mutate: func(s *corev1.PodSpec) { s.SecurityContext = &corev1.PodSecurityContext{} },
			check: func(t *testing.T, s *corev1.PodSpec) {
				assert.NotNil(t, s.SecurityContext, "security context should be preserved when not owned")
			},
		},
		{
			name: "Container.SecurityContext",
			mutate: func(s *corev1.PodSpec) {
				s.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: ptrBool(true)}
			},
			check: func(t *testing.T, s *corev1.PodSpec) {
				require.NotNil(t, s.Containers[0].SecurityContext,
					"container security context should be preserved when not owned")
				require.NotNil(t, s.Containers[0].SecurityContext.Privileged,
					"container privileged setting should be preserved when not owned")
				assert.True(t, *s.Containers[0].SecurityContext.Privileged,
					"container security context should be preserved when not owned")
			},
		},
		{
			name: "ImagePullSecrets",
			mutate: func(s *corev1.PodSpec) {
				s.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "regcred"}}
			},
			check: func(t *testing.T, s *corev1.PodSpec) {
				assert.Len(t, s.ImagePullSecrets, 1,
					"image pull secrets should be preserved when not owned")
			},
		},
		{
			name: "TopologySpreadConstraints",
			mutate: func(s *corev1.PodSpec) {
				s.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
					MaxSkew:           1,
					TopologyKey:       "zone",
					WhenUnsatisfiable: corev1.ScheduleAnyway,
				}}
			},
			check: func(t *testing.T, s *corev1.PodSpec) {
				assert.Len(t, s.TopologySpreadConstraints, 1,
					"topology spread constraints should be preserved when not owned")
			},
		},
	}
}

func clearOwnedPodSpecOptionals(s *corev1.PodSpec) {
	s.TerminationGracePeriodSeconds = nil
	s.Affinity = nil
	s.Tolerations = nil
	s.SecurityContext = nil
	s.Containers[0].SecurityContext = nil
	s.ImagePullSecrets = nil
	s.TopologySpreadConstraints = nil
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
