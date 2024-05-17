package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
)

func TestRenderEndpointUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		//
		// Endpoints
		//
		{
			name: "endpoints: endpoint-ips ok",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = false
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
			name: "endpoints: ready endpoint-ips ok",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = false
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
			name: "endpoints: wrong endpoint object name gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpoint.DeepCopy()
				e.SetName("dummy")
				c.eps = []corev1.Endpoints{*e}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = false
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
			name: "endpoints: wrong endpoint object namespace gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpoint.DeepCopy()
				e.SetNamespace("dummy")
				c.eps = []corev1.Endpoints{*e}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = false
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
			name: "endpoints: multiple endpoint objects ok",
			svcs: []corev1.Service{testutils.TestSvc},
			eps:  []corev1.Endpoints{testutils.TestEndpoint},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpoint.DeepCopy()
				e.SetName("dummy")
				c.eps = []corev1.Endpoints{testutils.TestEndpoint, *e}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = false
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
		//
		// EndpointSlice
		//
		{
			name: "endpointslice: endpoint-ips ok",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = true
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}

				// include not-ready addresses (default)
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
			name: "endpointslice: ready endpoint-ips ok",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = true
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}

				// exclude not-ready addresses
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
			name: "endpointslice: wrong endpoint object name gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpointSlice.DeepCopy()
				e.SetLabels(map[string]string{"kubernetes.io/service-name": "dummy"})
				c.esls = []discoveryv1.EndpointSlice{*e}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = true
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
			name: "endpointslice: wrong endpoint object namespace gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpointSlice.DeepCopy()
				e.SetNamespace("dummy")
				c.esls = []discoveryv1.EndpointSlice{*e}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = true
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
				config.EndpointSliceAvailable = false
			},
		},
		{
			name: "endpointslice: multiple endpoint objects ok",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpointSlice.DeepCopy()
				e.SetName("testendpointslice-ok-2")
				e.Endpoints = []discoveryv1.Endpoint{{
					Addresses: []string{"1.2.3.8"},
					Conditions: discoveryv1.EndpointConditions{
						Ready:   &testutils.TestTrue,
						Serving: &testutils.TestTrue,
					},
				}}
				c.esls = []discoveryv1.EndpointSlice{testutils.TestEndpointSlice, *e}
			},
			tester: func(t *testing.T, r *Renderer) {
				config.EndpointSliceAvailable = true
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
				assert.Len(t, addrs, 5, "endpoint addrs len ok")
				assert.Contains(t, addrs, "1.2.3.4", "addr-1 ok")
				assert.Contains(t, addrs, "1.2.3.5", "addr-2 ok")
				assert.Contains(t, addrs, "1.2.3.6", "addr-3 ok")
				assert.Contains(t, addrs, "1.2.3.7", "addr-4 ok")
				assert.Contains(t, addrs, "1.2.3.8", "addr-5 ok")
			},
		},
	})
}
