package renderer

import (
	// "context"
	//"fmt"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apiutil "k8s.io/apimachinery/pkg/util/intstr"

	// "k8s.io/apimachinery/pkg/types"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderDataplaneUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "default deployment render",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *DefaultRenderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})

				obj, err := r.generateDataplane(c)
				assert.NoError(t, err, "create deployment")

				deploy, ok := obj.(*appv1.Deployment)
				assert.True(t, ok, "deployment cast")

				assert.Equal(t, gw.GetName(), deploy.GetName(), "deployment name")
				assert.Equal(t, gw.GetNamespace(), deploy.GetNamespace(), "deployment namespace")

				labs := deploy.GetLabels()
				assert.Len(t, labs, 4, "labels len")
				v, ok := labs[opdefault.OwnedByLabelKey]
				assert.True(t, ok, "labels: owned-by")
				assert.Equal(t, opdefault.OwnedByLabelValue, v, "owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw label value")
				// label from the gateway
				v, ok = labs["dummy-label"]
				assert.True(t, ok, "labels: gateway label copied")
				assert.Equal(t, "dummy-value", v, "copied gateway label value")

				as := deploy.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwName, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")
				// annotation is gw-namespace/gw-name
				assert.Equal(t, store.GetObjectKey(gw), gwName, "related-gateway annotation")

				// check the label selector
				labelSelector := deploy.Spec.Selector
				assert.NotNil(t, labelSelector, "label selector")

				selector, err := metav1.LabelSelectorAsSelector(labelSelector)
				assert.NoError(t, err, "label selector convert")

				labelToMatch := labels.Merge(
					labels.Merge(
						labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
						labels.Set{opdefault.RelatedGatewayKey: gw.GetName()},
					),
					labels.Set{opdefault.RelatedGatewayNamespace: gw.GetNamespace()},
				)
				assert.True(t, selector.Matches(labelToMatch), "selector matched")

				// spec
				assert.NotNil(t, deploy.Spec.Replicas, "replicas notnil")
				assert.Equal(t, int32(3), *deploy.Spec.Replicas, "replicas")

				// pod template spec
				podTemplate := &deploy.Spec.Template
				labs = podTemplate.GetLabels()
				assert.Len(t, labs, 3, "labels len")
				v, ok = labs[opdefault.AppLabelKey]
				assert.True(t, ok, "pod labels: owned-by")
				assert.Equal(t, opdefault.AppLabelValue, v, "pod owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "pod related-gw label")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "pod related-gw-namespace label")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw-namespace label value")

				// deployment selector matches pod template
				assert.True(t, selector.Matches(labels.Set(labs)), "selector matched")

				podSpec := &podTemplate.Spec
				assert.True(t, len(podSpec.Containers) >= 1, "containers len")

				// template must be such that the first pod is the stunnerd container
				container := podSpec.Containers[0]
				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, container.Name, "container 1 name")
				assert.Equal(t, "testimage-1", container.Image, "container 1 image")
				assert.Equal(t, []string{"testcommand-1"}, container.Command, "container 1 command")
				assert.Equal(t, []string{"arg-1", "arg-2"}, container.Args, "container 1 args")
				assert.Equal(t, testutils.TestResourceLimit, container.Resources.Limits, "container 1 - resource limits")
				assert.Equal(t, testutils.TestResourceRequest, container.Resources.Requests, "container 1 - resource req")
				assert.Equal(t, corev1.PullAlways, container.ImagePullPolicy, "container 1 - pull policy")

				action := corev1.HTTPGetAction{
					Path:   "/live",
					Port:   apiutil.FromInt(8086),
					Scheme: "HTTP",
				}
				probe := corev1.Probe{
					ProbeHandler:  corev1.ProbeHandler{HTTPGet: &action},
					PeriodSeconds: 15, SuccessThreshold: 1, FailureThreshold: 3, TimeoutSeconds: 1,
				}
				assert.Equal(t, probe, *container.LivenessProbe, "container 1 - liveness probe")

				action = corev1.HTTPGetAction{
					Path:   "/ready",
					Port:   apiutil.FromInt(8086),
					Scheme: "HTTP",
				}
				probe = corev1.Probe{
					ProbeHandler:  corev1.ProbeHandler{HTTPGet: &action},
					PeriodSeconds: 15, SuccessThreshold: 1, FailureThreshold: 3, TimeoutSeconds: 1,
				}
				assert.Equal(t, probe, *container.ReadinessProbe, "container 1 - readiness probe")

				assert.Len(t, container.Env, 5, "container 1 - env len")
				podAddrFieldSelector := corev1.ObjectFieldSelector{FieldPath: "status.podIP"}
				podAddrEnvVarSource := corev1.EnvVarSource{FieldRef: &podAddrFieldSelector}
				assert.Contains(t, container.Env,
					corev1.EnvVar{Name: "STUNNER_ADDR", ValueFrom: &podAddrEnvVarSource},
					"container 1 - env: addr")
				nodeNameFieldSelector := corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}
				nodeNameEnvVarSource := corev1.EnvVarSource{FieldRef: &nodeNameFieldSelector}
				assert.Contains(t, container.Env, corev1.EnvVar{Name: stnrconfv1.DefaultEnvVarNodeName,
					ValueFrom: &nodeNameEnvVarSource}, "container 1 - env: nodename")
				assert.Contains(t, container.Env, corev1.EnvVar{Name: stnrconfv1.DefaultEnvVarName,
					Value: gw.GetName()}, "container 1 - env: name")
				assert.Contains(t, container.Env, corev1.EnvVar{Name: stnrconfv1.DefaultEnvVarNamespace,
					Value: gw.GetNamespace()}, "container 1 - env: namespace")
				cdsAddr := &url.URL{Scheme: "http", Host: config.ConfigDiscoveryAddress}
				assert.Contains(t, container.Env, corev1.EnvVar{Name: stnrconfv1.DefaultEnvVarConfigOrigin,
					Value: cdsAddr.String()}, "container 1 - env: origin")

				// remainder
				assert.NotNil(t, podSpec.TerminationGracePeriodSeconds, "termination grace ptr")
				assert.Equal(t, testutils.TestTerminationGrace, *podSpec.TerminationGracePeriodSeconds,
					"termination grace")
				assert.True(t, podSpec.HostNetwork, "hostnetwork")
				assert.Nil(t, podSpec.Affinity, "affinity")
			},
		},
		{
			name: "override render",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				dp := c.dps[0].DeepCopy()
				dp.Spec.DisableHealthCheck = true
				dp.Spec.EnableMetricsEnpoint = true
				dp.Spec.HostNetwork = false
				dp.Spec.ImagePullSecrets = []corev1.LocalObjectReference{{
					Name: "testpullsecret1",
				}, {
					Name: "testpullsecret2",
				}}
				dp.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{{
					MaxSkew: int32(12),
				}, {
					MaxSkew: int32(21),
				}}
				runAsNonRoot := true
				dp.Spec.SecurityContext = &corev1.PodSecurityContext{RunAsNonRoot: &runAsNonRoot}
				dp.Spec.ContainerSecurityContext = &corev1.SecurityContext{RunAsNonRoot: &runAsNonRoot}
				dp.Spec.Labels = map[string]string{"dummy-label-key": "dummy-value", "app": "conflicted-value"}
				dp.Spec.Annotations = map[string]string{"dummy-annotation-key": "dummy-value",
					opdefault.RelatedGatewayKey: "conflicted-value"}

				c.dps = []stnrgwv1.Dataplane{*dp}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})
				c.dp, err = getDataplane(c)
				assert.NoError(t, err, "dataplanefound")

				obj, err := r.generateDataplane(c)
				assert.NoError(t, err, "create deployment")

				deploy, ok := obj.(*appv1.Deployment)
				assert.True(t, ok, "deployment cast")

				assert.Equal(t, gw.GetName(), deploy.GetName(), "deployment name")
				assert.Equal(t, gw.GetNamespace(), deploy.GetNamespace(), "deployment namespace")

				labs := deploy.GetLabels()
				assert.Len(t, labs, 4, "labels len")
				v, ok := labs[opdefault.OwnedByLabelKey]
				assert.True(t, ok, "labels: owned-by")
				assert.Equal(t, opdefault.OwnedByLabelValue, v, "owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw label value")
				// label from the gateway
				v, ok = labs["dummy-label"]
				assert.True(t, ok, "labels: gateway label copied")
				assert.Equal(t, "dummy-value", v, "copied gateway label value")

				as := deploy.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwName, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")
				// annotation is gw-namespace/gw-name
				assert.Equal(t, store.GetObjectKey(gw), gwName, "related-gateway annotation")

				// check the label selector
				labelSelector := deploy.Spec.Selector
				assert.NotNil(t, labelSelector, "label selector")

				selector, err := metav1.LabelSelectorAsSelector(labelSelector)
				assert.NoError(t, err, "label selector convert")

				labelToMatch := labels.Merge(
					labels.Merge(
						labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
						labels.Set{opdefault.RelatedGatewayKey: gw.GetName()},
					),
					labels.Set{opdefault.RelatedGatewayNamespace: gw.GetNamespace()},
				)
				assert.True(t, selector.Matches(labelToMatch), "selector matched")

				// spec
				assert.NotNil(t, deploy.Spec.Replicas, "replicas notnil")
				assert.Equal(t, int32(3), *deploy.Spec.Replicas, "replicas")

				// pod template spec
				podTemplate := &deploy.Spec.Template
				labs = podTemplate.GetLabels()
				assert.Len(t, labs, 4, "labels len")
				v, ok = labs[opdefault.AppLabelKey]
				assert.True(t, ok, "pod labels: owned-by")
				assert.Equal(t, opdefault.AppLabelValue, v, "pod owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "pod related-gw label")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "pod related-gw-namespace label")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw-namespace label value")
				v, ok = labs["dummy-label-key"]
				assert.True(t, ok, "pod labels from dataplane spec labels")
				assert.Equal(t, "dummy-value", v, "pod labels from Dataplane spec labels value")

				as = podTemplate.GetAnnotations()
				assert.Len(t, as, 2, "pod anns len")
				v, ok = as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "pod anns: related-gw")
				assert.Equal(t, fmt.Sprintf("%s/%s", gw.GetNamespace(), gw.GetName()), v,
					"pod anns: related-gw value")
				v, ok = as["dummy-annotation-key"]
				assert.True(t, ok, "pod anns from dataplane spec key")
				assert.Equal(t, "dummy-value", v, "pod anns from dataplane spec label")

				// deployment selector matches pod template
				assert.True(t, selector.Matches(labels.Set(labs)), "selector matched")

				podSpec := &podTemplate.Spec
				assert.True(t, len(podSpec.Containers) >= 1, "containers len")

				// template must be such that the first pod is the stunnerd container
				container := podSpec.Containers[0]
				assert.Equal(t, opdefault.DefaultStunnerdInstanceName, container.Name, "container 1 name")
				assert.Equal(t, "testimage-1", container.Image, "container 1 image")
				assert.Equal(t, []string{"testcommand-1"}, container.Command, "container 1 command")
				assert.Equal(t, []string{"arg-1", "arg-2"}, container.Args, "container 1 args")
				assert.Equal(t, testutils.TestResourceLimit, container.Resources.Limits, "container 1 - resource limits")
				assert.Equal(t, testutils.TestResourceRequest, container.Resources.Requests, "container 1 - resource req")
				assert.Equal(t, corev1.PullAlways, container.ImagePullPolicy, "container 1 - readiness probe")
				assert.Nil(t, container.LivenessProbe, "container 1 - liveness probe")
				assert.Nil(t, container.ReadinessProbe, "container 1 - liveness probe")
				assert.NotNil(t, container.SecurityContext, "container security context ptr")
				assert.NotNil(t, container.SecurityContext.RunAsNonRoot, "container security context bool ptr")
				assert.True(t, *container.SecurityContext.RunAsNonRoot, "container security context bool")

				// remainder
				assert.NotNil(t, podSpec.TerminationGracePeriodSeconds, "termination grace ptr")
				assert.Equal(t, testutils.TestTerminationGrace, *podSpec.TerminationGracePeriodSeconds, "termination grace")
				assert.False(t, podSpec.HostNetwork, "hostnetwork")
				assert.Nil(t, podSpec.Affinity, "affinity")
				assert.Len(t, podSpec.ImagePullSecrets, 2, "image pull secrets len")
				assert.Equal(t, "testpullsecret1", podSpec.ImagePullSecrets[0].Name, "image pull secret 1")
				assert.Equal(t, "testpullsecret2", podSpec.ImagePullSecrets[1].Name, "image pull secret 2")
				assert.Len(t, podSpec.TopologySpreadConstraints, 2, "topopology spread constraints len")
				assert.Equal(t, int32(12), podSpec.TopologySpreadConstraints[0].MaxSkew, "topopology spread constraints 1")
				assert.Equal(t, int32(21), podSpec.TopologySpreadConstraints[1].MaxSkew, "topopology spread constraints 2")
				assert.NotNil(t, podSpec.SecurityContext, "pod security context ptr")
				assert.NotNil(t, podSpec.SecurityContext.RunAsNonRoot, "pod security context bool ptr")
				assert.True(t, *podSpec.SecurityContext.RunAsNonRoot, "pod security context bool")
			},
		},
		{
			name: "override labels",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			dps:  []stnrgwv1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetLabels(map[string]string{
					"stunner.l7mp.io/owned-by": "dummy-owner",          // will be overwritten on the deployment
					"valid-gw-label":           "valid-gw-label-value", // will appear on the deployment
				})
				gw.SetAnnotations(map[string]string{
					"stunner.l7mp.io/related-gateway-name": "dummy-ns/dummy-name", // will be overwritten on the deployment/pod
					"valid-gw-ann":                         "valid-gw-ann-value",  // will appear on the deployment/pod
				})
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *DefaultRenderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]
				c.gws.ResetGateways([]*gwapiv1.Gateway{gw})
				c.dp, err = getDataplane(c)
				assert.NoError(t, err, "dataplanefound")

				obj, err := r.generateDataplane(c)
				assert.NoError(t, err, "create deployment")

				deploy, ok := obj.(*appv1.Deployment)
				assert.True(t, ok, "deployment cast")

				assert.Equal(t, gw.GetName(), deploy.GetName(), "deployment name")
				assert.Equal(t, gw.GetNamespace(), deploy.GetNamespace(), "deployment namespace")

				labs := deploy.GetLabels()
				assert.Len(t, labs, 4, "labels len")

				// mandatory labels
				v, ok := labs[opdefault.OwnedByLabelKey]
				assert.True(t, ok, "labels: owned-by")
				assert.Equal(t, opdefault.OwnedByLabelValue, v, "owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "labels: related")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw label value")

				// label from the gateway
				v, ok = labs["valid-gw-label"]
				assert.True(t, ok, "labels: gw label copied")
				assert.Equal(t, "valid-gw-label-value", v, "copied gw label value")

				as := deploy.GetAnnotations()
				assert.Len(t, as, 2, "annotations len")

				// mandatory annotations
				gwName, ok := as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")
				// annotation is gw-namespace/gw-name
				assert.Equal(t, store.GetObjectKey(gw), gwName, "related-gateway annotation")

				// optional annotations
				a, ok := as["valid-gw-ann"]
				assert.True(t, ok, "annotations: valid gw ann")
				assert.Equal(t, "valid-gw-ann-value", a, "valid gateway annotation value")

				// check the label selector
				labelSelector := deploy.Spec.Selector
				assert.NotNil(t, labelSelector, "label selector")

				selector, err := metav1.LabelSelectorAsSelector(labelSelector)
				assert.NoError(t, err, "label selector convert")

				labelToMatch := labels.Merge(
					labels.Merge(
						labels.Set{opdefault.AppLabelKey: opdefault.AppLabelValue},
						labels.Set{opdefault.RelatedGatewayKey: gw.GetName()},
					),
					labels.Set{opdefault.RelatedGatewayNamespace: gw.GetNamespace()},
				)
				assert.True(t, selector.Matches(labelToMatch), "selector matched")

				// pod template spec
				podTemplate := &deploy.Spec.Template
				labs = podTemplate.GetLabels()

				// only the manadatory labels
				assert.Len(t, labs, 3, "labels len")
				v, ok = labs[opdefault.AppLabelKey]
				assert.True(t, ok, "pod labels: owned-by")
				assert.Equal(t, opdefault.AppLabelValue, v, "pod owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "pod related-gw label")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")
				v, ok = labs[opdefault.RelatedGatewayNamespace]
				assert.True(t, ok, "pod related-gw-namespace label")
				assert.Equal(t, gw.GetNamespace(), v, "related-gw-namespace label value")

				// deployment selector matches pod template
				assert.True(t, selector.Matches(labels.Set(labs)), "selector matched")

				as = podTemplate.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")

				// mandatory annotations
				gwName, ok = as[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "annotations: related gw")
				// annotation is gw-namespace/gw-name
				assert.Equal(t, store.GetObjectKey(gw), gwName, "related-gateway annotation")
			},
		},
	})
}
