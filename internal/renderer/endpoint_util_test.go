package renderer

import (
	// "context"
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
)

func TestRenderEndpointUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "endpoint-ips ok",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, err := getEndpointAddrs(n, false)

				assert.Nil(t, err, "no error")
				assert.NotEmpty(t, addrs, "endpoint addrs found")
				assert.Len(t, addrs, 4, "endpoint addrs len ok")
				assert.Contains(t, addrs, "1.2.3.4", "addr-1 ok")
				assert.Contains(t, addrs, "1.2.3.5", "addr-2 ok")
				assert.Contains(t, addrs, "1.2.3.6", "addr-3 ok")
				assert.Contains(t, addrs, "1.2.3.7", "addr-4 ok")
			},
		},
		{
			name: "ready endpoint-ips ok",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, err := getEndpointAddrs(n, true)

				assert.Nil(t, err, "no error")
				assert.NotEmpty(t, addrs, "endpoint addrs found")
				assert.Len(t, addrs, 3, "endpoint addrs len ok")
				assert.Contains(t, addrs, "1.2.3.4", "addr-1 ok")
				assert.Contains(t, addrs, "1.2.3.5", "addr-2 ok")
				assert.Contains(t, addrs, "1.2.3.7", "addr-4 ok")
			},
		},
		{
			name: "wrong endpoint object name gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpoint.DeepCopy()
				e.SetName("dummy")
				c.eps = []corev1.Endpoints{*e}
			},
			tester: func(t *testing.T, r *Renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, err := getEndpointAddrs(n, false)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")
				assert.Empty(t, addrs, "endpoint addrs found")
			},
		},
		{
			name: "wrong endpoint object namespace gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpoint.DeepCopy()
				e.SetNamespace("dummy")
				c.eps = []corev1.Endpoints{*e}
			},
			tester: func(t *testing.T, r *Renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, err := getEndpointAddrs(n, false)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")
				assert.Empty(t, addrs, "endpoint addrs found")
			},
		},
		{
			name: "multiple endpoint objects ok",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpoint.DeepCopy()
				e.SetName("dummy")
				c.eps = []corev1.Endpoints{testutils.TestEndpoint, *e}
			},
			tester: func(t *testing.T, r *Renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, err := getEndpointAddrs(n, false)

				assert.Nil(t, err, "no error")
				assert.NotEmpty(t, addrs, "endpoint addrs found")
				assert.Len(t, addrs, 4, "endpoint addrs len ok")
				assert.Contains(t, addrs, "1.2.3.4", "addr-1 ok")
				assert.Contains(t, addrs, "1.2.3.5", "addr-2 ok")
				assert.Contains(t, addrs, "1.2.3.6", "addr-3 ok")
				assert.Contains(t, addrs, "1.2.3.7", "addr-4 ok")
			},
		},
	})
}
