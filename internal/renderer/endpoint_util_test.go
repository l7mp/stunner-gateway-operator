package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
)

func TestRenderEndpointUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		//
		// EndpointSlice
		//
		{
			name: "endpointslice: endpoint-ips ok",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}

				// every address, ready or not
				addrs, _, _, err := getEndpointAddrs(n, nil)

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
			tester: func(t *testing.T, r *renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}

				// dial the ready addresses only
				_, dial, _, err := getEndpointAddrs(n, nil)

				assert.Nil(t, err, "no error")
				assert.Equal(t, map[string]bool{"1.2.3.4": true, "1.2.3.5": true}, dial, "ready addrs dialed")
			},
		},
		{
			name: "endpointslice: serving terminating endpoint-ips dialed when none is ready",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpointSlice.DeepCopy()
				e.Endpoints = []discoveryv1.Endpoint{{
					Addresses: []string{"1.2.3.4"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &testutils.TestFalse, Serving: &testutils.TestTrue, Terminating: &testutils.TestTrue,
					},
				}, {
					Addresses: []string{"1.2.3.5"},
					Conditions: discoveryv1.EndpointConditions{
						Ready: &testutils.TestFalse, Serving: &testutils.TestFalse, Terminating: &testutils.TestTrue,
					},
				}}
				c.esls = []discoveryv1.EndpointSlice{*e}
			},
			tester: func(t *testing.T, r *renderer) {
				n := types.NamespacedName{Namespace: testutils.TestSvc.GetNamespace(), Name: testutils.TestSvc.GetName()}
				addrs, dial, _, err := getEndpointAddrs(n, nil)

				assert.Nil(t, err, "no error")
				assert.Len(t, addrs, 2, "every addr")
				assert.True(t, dial["1.2.3.4"], "the serving terminating addr dialed")
				assert.False(t, dial["1.2.3.5"], "the unready terminating addr not dialed")
			},
		},
		{
			name: "endpointslice: wrong service-name label of the EndpointSlice gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpointSlice.DeepCopy()
				e.SetLabels(map[string]string{"kubernetes.io/service-name": "dummy"})
				c.esls = []discoveryv1.EndpointSlice{*e}
			},
			tester: func(t *testing.T, r *renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, _, _, err := getEndpointAddrs(n, nil)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")
				assert.Empty(t, addrs, "endpoint addrs found")
			},
		},
		{
			name: "endpointslice: wrong namespace of the EndpointSlice gives empty addr list",
			svcs: []corev1.Service{testutils.TestSvc},
			esls: []discoveryv1.EndpointSlice{testutils.TestEndpointSlice},
			prep: func(c *renderTestConfig) {
				e := testutils.TestEndpointSlice.DeepCopy()
				e.SetNamespace("dummy")
				c.esls = []discoveryv1.EndpointSlice{*e}
			},
			tester: func(t *testing.T, r *renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, _, _, err := getEndpointAddrs(n, nil)

				assert.NotNil(t, err, "error")
				assert.True(t, IsNonCritical(err), "non-critical error")
				assert.True(t, IsNonCriticalError(err, EndpointNotFound), "endpoint not found error")
				assert.Empty(t, addrs, "endpoint addrs found")
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
			tester: func(t *testing.T, r *renderer) {
				svcs := store.Services.GetAll()
				assert.NotEmpty(t, svcs, "svcs exist")
				assert.Len(t, svcs, 1, "svcs len ok")

				n := types.NamespacedName{
					Namespace: svcs[0].GetNamespace(),
					Name:      svcs[0].GetName(),
				}
				addrs, _, _, err := getEndpointAddrs(n, nil)

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
