package renderer

import (
	// "context"
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderNodeUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name:  "node-ip ok",
			cls:   []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:   []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:    []gatewayv1alpha2.UDPRoute{},
			svcs:  []corev1.Service{testutils.TestSvc},
			nodes: []corev1.Node{testutils.TestNode},
			prep:  func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				addr := getFirstNodeAddr()
				assert.NotEmpty(t, addr, "public node-addr found")
				assert.Equal(t, "1.2.3.4", addr, "public addr ok")
			},
		},
		{
			name:  "second valid node-ip ok",
			cls:   []gatewayv1alpha2.GatewayClass{testutils.TestGwClass},
			cfs:   []stunnerv1alpha1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gatewayv1alpha2.Gateway{testutils.TestGw},
			rs:    []gatewayv1alpha2.UDPRoute{},
			svcs:  []corev1.Service{testutils.TestSvc},
			nodes: []corev1.Node{testutils.TestNode},
			prep: func(c *renderTestConfig) {
				n1 := testutils.TestNode.DeepCopy()
				// remove the external address
				n1.Status.Addresses = n1.Status.Addresses[:len(n1.Status.Addresses)-1]
				n2 := testutils.TestNode.DeepCopy()
				n2.SetName("node-2")
				c.nodes = []corev1.Node{*n1, *n2}
			},
			tester: func(t *testing.T, r *Renderer) {
				addr := getFirstNodeAddr()
				assert.NotEmpty(t, addr, "public node-addr found")
				assert.Equal(t, "1.2.3.4", addr, "public addr ok")
			},
		},
		{
			name:  "invalid node-ip gives empty string",
			nodes: []corev1.Node{testutils.TestNode},
			prep: func(c *renderTestConfig) {
				n1 := testutils.TestNode.DeepCopy()
				// remove the external address
				n1.Status.Addresses = []corev1.NodeAddress{}
				c.nodes = []corev1.Node{*n1}
			},
			tester: func(t *testing.T, r *Renderer) {
				addr := getFirstNodeAddr()
				assert.Empty(t, addr, "public node-addr empty")
			},
		},
	})
}
