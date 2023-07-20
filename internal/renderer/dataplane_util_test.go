package renderer

import (
	// "context"
	//"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	// "k8s.io/apimachinery/pkg/types"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func TestRenderDataplaneUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "default deployment render",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{testutils.TestGw},
			dps:  []stnrv1a1.Dataplane{testutils.TestDataplane},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, gws: store.NewGatewayStore(), log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", c.gwConf.GetName(),
					"gatewayconfig name")

				c.update = event.NewEventUpdate(0)
				assert.NotNil(t, c.update, "update event create")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]
				c.gws.ResetGateways([]*gwapiv1a2.Gateway{gw})

				deploy, err := r.createDeployment(c, gw.GetName(), gw.GetNamespace())
				assert.NoError(t, err, "create deployment")

				assert.Equal(t, gw.GetName(), deploy.GetName(), "deployment name")
				assert.Equal(t, gw.GetNamespace(), deploy.GetNamespace(), "deployment namespace")

				labs := deploy.GetLabels()
				assert.Len(t, labs, 2, "labels len")
				v, ok := labs[opdefault.OwnedByLabelKey]
				assert.True(t, ok, "labels: owned-by")
				assert.Equal(t, opdefault.OwnedByLabelValue, v, "owned-by label value")
				// label from the dataplane object
				v, ok = labs["dummy-label"]
				assert.True(t, ok, "labels: dataplane label copied")
				assert.Equal(t, "dummy-value", v, "copied dataplane label value")

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

				// match "opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue" AND
				// "stunner.l7mp.io/related-gateway-name=<gateway-name>"
				labelToMatch := labels.Merge(
					labels.Set{opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue},
					labels.Set{opdefault.RelatedGatewayKey: gw.GetName()},
				)
				assert.True(t, selector.Matches(labelToMatch), "selector matched")

				// spec
				assert.NotNil(t, deploy.Spec.Replicas, "replicas notnil")
				assert.Equal(t, int32(3), *deploy.Spec.Replicas, "replicas")
				assert.NotNil(t, deploy.Spec.Strategy, "strategy notnil")
				assert.Equal(t, testutils.TestDeployStrategy, deploy.Spec.Strategy, "strategy")

				// pod template spec
				podTemplate := &deploy.Spec.Template
				labs = podTemplate.GetLabels()
				assert.Len(t, labs, 2, "labels len")
				v, ok = labs[opdefault.OwnedByLabelKey]
				assert.True(t, ok, "pod labels: owned-by")
				assert.Equal(t, opdefault.OwnedByLabelValue, v, "pod owned-by label value")
				v, ok = labs[opdefault.RelatedGatewayKey]
				assert.True(t, ok, "pod related-gw label")
				assert.Equal(t, gw.GetName(), v, "related-gw label value")

				// deployment selector matches pod template
				assert.True(t, selector.Matches(labels.Set(labs)), "selector matched")

				podSpec := &podTemplate.Spec

				assert.Len(t, podSpec.Containers, 2, "contianers len")

				container := podSpec.Containers[0]

				assert.Equal(t, "testcontainer-1", container.Name, "container 1 name")
				assert.Equal(t, "testimage-1", container.Image, "container 1 image")
				assert.Equal(t, []string{"testcommand-1-1", "testcommand-1-2"}, container.Command, "container 1 command")
				assert.Equal(t, []string{"testarg-1-1", "testarg-1-2"}, container.Args, "container 1 args")

				ports := container.Ports
				assert.Len(t, ports, 2, "contianer 1 ports len")
				port := ports[0]
				assert.Equal(t, "testport-1-1", port.Name, "container 1 - port 1 - name")
				assert.Equal(t, int32(1), port.ContainerPort, "container 1 - port 1 - port")
				assert.Equal(t, corev1.ProtocolUDP, port.Protocol, "container 1 - port 1 - protocol")
				port = ports[1]
				assert.Equal(t, "testport-1-2", port.Name, "container 1 - port 2 - name")
				assert.Equal(t, int32(2), port.ContainerPort, "container 1 - port 2 - port")
				assert.Equal(t, corev1.ProtocolTCP, port.Protocol, "container 1 - port 2 - protocol")

				assert.Equal(t, []corev1.EnvFromSource{}, container.EnvFrom, "container 1 - envFrom")

				assert.Len(t, container.Env, 2, "container 1 env len")
				env := container.Env[0]
				assert.Equal(t, "TEST_ENV_1", env.Name, "container 1 - env 1 - name")
				assert.Equal(t, "test-env-val", env.Value, "container 1 - env 1 - value")
				env = container.Env[1]
				assert.Equal(t, "TEST_ENV_2", env.Name, "container 1 - env 2 - name")
				assert.NotNil(t, env.ValueFrom, "container 1 - env 2 - valueFrom ptr")
				assert.Equal(t, testutils.TestEnvEnvVarSource, *env.ValueFrom, "container 1 - env 2 - value")

				assert.Equal(t, testutils.TestResourceLimit, container.Resources.Limits, "container 1 - resource limits")
				assert.Equal(t, testutils.TestResourceRequest, container.Resources.Requests, "container 1 - resource req")

				assert.Len(t, container.VolumeMounts, 1, "contianer 1 - volume mounts")
				assert.Equal(t, "testvolume-name", container.VolumeMounts[0].Name, "container 1 - volume mount - name")
				assert.Equal(t, true, container.VolumeMounts[0].ReadOnly, "container 1 - volume mount - readonly")
				assert.Equal(t, "/tmp/mount", container.VolumeMounts[0].MountPath, "container 1 - volume mount - mount-path")

				assert.NotNil(t, container.LivenessProbe, "container 1 - liveness probe ptr")
				assert.Equal(t, testutils.TestProbe, *container.LivenessProbe, "container 1 - liveness probe")
				assert.NotNil(t, container.ReadinessProbe, "container 1 - readiness probe ptr")
				assert.Equal(t, testutils.TestProbe, *container.ReadinessProbe, "container 1 - readiness probe")

				assert.Equal(t, corev1.PullAlways, container.ImagePullPolicy, "container 1 - readiness probe")
				assert.Nil(t, container.SecurityContext, "container 1 - security context")

				container = podSpec.Containers[1]

				assert.Equal(t, "testcontainer-2", container.Name, "container 2 name")
				assert.Equal(t, "testimage-2", container.Image, "container 2 image")
				assert.Equal(t, []string{"testcommand-2-1", "testcommand-2-2"}, container.Command, "container 2 command")
				assert.Equal(t, []string{"testarg-2-1", "testarg-2-2"}, container.Args, "container 2 args")

				ports = container.Ports
				assert.Len(t, ports, 2, "contianer 2 ports len")
				port = ports[0]
				assert.Equal(t, "testport-2-1", port.Name, "container 2 - port 1 - name")
				assert.Equal(t, int32(1), port.ContainerPort, "container 2 - port 1 - port")
				assert.Equal(t, corev1.ProtocolUDP, port.Protocol, "container 2 - port 1 - protocol")
				port = ports[1]
				assert.Equal(t, "testport-2-2", port.Name, "container 2 - port 2 - name")
				assert.Equal(t, int32(2), port.ContainerPort, "container 2 - port 2 - port")
				assert.Equal(t, corev1.ProtocolTCP, port.Protocol, "container 2 - port 2 - protocol")

				assert.Equal(t, []corev1.EnvFromSource{}, container.EnvFrom, "container 2 - envFrom")
				assert.Equal(t, []corev1.EnvVar{}, container.Env, "container 2 - envFrom")

				assert.NotNil(t, container.LivenessProbe, "container 2 - liveness probe ptr")
				assert.Equal(t, testutils.TestProbe, *container.LivenessProbe, "container 2 - liveness probe")
				assert.NotNil(t, container.ReadinessProbe, "container 2 - readiness probe ptr")
				assert.Equal(t, testutils.TestProbe, *container.ReadinessProbe, "container 2 - readiness probe")

				assert.Len(t, container.VolumeMounts, 0, "contianer 2 - volume mounts")
				assert.NotNil(t, container.LivenessProbe, "container 2 - liveness probe ptr")
				assert.Equal(t, testutils.TestProbe, *container.LivenessProbe, "container 2 - liveness probe")
				assert.NotNil(t, container.ReadinessProbe, "container 2 - readiness probe ptr")
				assert.Equal(t, testutils.TestProbe, *container.ReadinessProbe, "container 2 - readiness probe")

				assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy, "container 2 - readiness probe")
				assert.Nil(t, container.SecurityContext, "container 2 - security context")

				// remainder
				assert.NotNil(t, podSpec.TerminationGracePeriodSeconds, "termination grace ptr")
				assert.Equal(t, testutils.TestTerminationGrace, *podSpec.TerminationGracePeriodSeconds, "termination grace")
				assert.True(t, podSpec.HostNetwork, "hostnetwork")
				assert.Nil(t, podSpec.Affinity, "affinity")

			},
		},
	})
}
