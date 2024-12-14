package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func TestRenderNodeUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name:  "node-ip ok",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			svcs:  []corev1.Service{testutils.TestSvc},
			nodes: []corev1.Node{testutils.TestNode},
			prep:  func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *DefaultRenderer) {
				addr := getFirstNodeAddr()
				assert.NotEmpty(t, addr, "public node-addr found")
				assert.Equal(t, "1.2.3.4", addr, "public addr ok")
			},
		},
		{
			name:  "second valid node-ip ok",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
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
			tester: func(t *testing.T, r *DefaultRenderer) {
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
			tester: func(t *testing.T, r *DefaultRenderer) {
				addr := getFirstNodeAddr()
				assert.Empty(t, addr, "public node-addr empty")
			},
		},
	})
}
