package renderer

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

// Config rendering pipeline benchmarks:
//
// # Run with longer time for more accurate results
// go test -bench=BenchmarkRenderPipeline ./internal/renderer -benchtime=5 -benchmem -run=^$
//
// # Using go tool pprof directly (modern Go versions)
// go test -bench=BenchmarkRenderPipeline -benchtime=5s -run=^$ -cpuprofile=cpu.prof -memprofile=mem
// go tool pprof -http=:8080 cpu.prof

// generateGateway creates a unique gateway based on the test template.
func generateGateway(index int) gwapiv1.Gateway {
	gw := testutils.TestGw.DeepCopy()
	gw.Name = fmt.Sprintf("gateway-%d", index)
	gw.Namespace = fmt.Sprintf("testnamespace-%d", index)
	return *gw
}

// generateUDPRoute creates a unique UDP route based on the test template.
func generateUDPRoute(index int) stnrgwv1.UDPRoute {
	route := testutils.TestUDPRoute.DeepCopy()
	route.Name = fmt.Sprintf("udproute-%d", index)
	route.Namespace = fmt.Sprintf("testnamespace-%d", index)
	// Update parent ref to match the gateway
	route.Spec.ParentRefs[0].Name = gwapiv1.ObjectName(fmt.Sprintf("gateway-%d", index))
	// Update backend ref to match the service
	route.Spec.Rules[0].BackendRefs[0].Name = gwapiv1.ObjectName(fmt.Sprintf("testservice-%d", index))
	return *route
}

// generateService creates a unique service based on the test template.
func generateService(index int) corev1.Service {
	svc := testutils.TestSvc.DeepCopy()
	svc.Name = fmt.Sprintf("testservice-%d", index)
	svc.Namespace = fmt.Sprintf("testnamespace-%d", index)
	// Update owner reference to match the gateway.
	svc.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: gwapiv1.GroupVersion.String(),
		Kind:       "Gateway",
		UID:        "test-uid",
		Name:       fmt.Sprintf("gateway-%d", index),
	}})
	svc.Spec.ClusterIP = fmt.Sprintf("10.0.%d.%d", index/256, index%256)
	return *svc
}

// generateEndpointSlice creates a unique endpoint slice based on the test template.
func generateEndpointSlice(index int) discoveryv1.EndpointSlice {
	esl := testutils.TestEndpointSlice.DeepCopy()
	esl.Name = fmt.Sprintf("testendpointslice-%d", index)
	esl.Namespace = fmt.Sprintf("testnamespace-%d", index)
	// Update label to bind to the service.
	esl.Labels["kubernetes.io/service-name"] = fmt.Sprintf("testservice-%d", index)
	return *esl
}

// benchmarkSetup prepares the stores with N gateways, routes, services, and endpoint slices.
func benchmarkSetup(n int) {
	// Flush all stores.
	store.GatewayClasses.Flush()
	store.GatewayConfigs.Flush()
	store.Gateways.Flush()
	store.UDPRoutes.Flush()
	store.Services.Flush()
	store.EndpointSlices.Flush()
	store.Dataplanes.Flush()

	// Add the single GatewayClass and GatewayConfig.
	store.GatewayClasses.Upsert(&testutils.TestGwClass)
	store.GatewayConfigs.Upsert(&testutils.TestGwConfig)
	store.Dataplanes.Upsert(&testutils.TestDataplane)

	// Generate and add N gateways, routes, services, and endpoint slices.
	for i := 0; i < n; i++ {
		gw := generateGateway(i)
		store.Gateways.Upsert(&gw)

		route := generateUDPRoute(i)
		store.UDPRoutes.Upsert(&route)

		svc := generateService(i)
		store.Services.Upsert(&svc)

		esl := generateEndpointSlice(i)
		store.EndpointSlices.Upsert(&esl)
	}
}

// BenchmarkRenderPipeline benchmarks the rendering pipeline with varying numbers of gateways.
func BenchmarkRenderPipeline(b *testing.B) {
	// Test with N=1,2,4,8,16,32,64,128,256,512 gateways.
	sizes := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512}

	for _, n := range sizes {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			// Setup: prepare stores with N resources.
			benchmarkSetup(n)

			// Configure managed mode with EDS enabled (like the reference test).
			config.DataplaneMode = config.DataplaneModeManaged
			config.EnableEndpointDiscovery = true
			config.EnableRelayToClusterIP = true
			config.EndpointSliceAvailable = true

			// Create the renderer.
			r := NewDefaultRenderer(RendererConfig{
				Scheme: scheme,
				Logger: log.WithName("benchmark-renderer"),
			}).(*renderer)

			// Start the renderer.
			err := r.Start(b.Context())
			if err != nil {
				b.Fatalf("failed to start renderer: %v", err)
			}

			// Prepare the render context (outside the benchmark loop).
			gc, err := r.getGatewayClass()
			if err != nil {
				b.Fatalf("failed to get gateway class: %v", err)
			}

			gwConf, err := r.getGatewayConfig4Class(&RenderContext{gc: gc, log: log})
			if err != nil {
				b.Fatalf("failed to get gateway config: %v", err)
			}

			gws := r.getGateways4Class(&RenderContext{gc: gc, log: log})
			if len(gws) != n {
				b.Fatalf("expected %d gateways, got %d", n, len(gws))
			}

			// Reset the benchmark timer to exclude setup time.
			b.ResetTimer()

			// Run the benchmark: measure how many full rendering cycles we can do.
			for i := 0; i < b.N; i++ {
				// Create a fresh render context for each iteration.
				c := &RenderContext{
					gc:     gc,
					gwConf: gwConf,
					gws:    store.NewGatewayStore(),
					log:    log,
					update: event.NewEventUpdate(0),
				}

				// Reset gateways in the context.
				c.gws.ResetGateways(gws)

				// Perform the rendering.
				err := r.renderForGateways(c)
				if err != nil {
					b.Fatalf("render failed: %v", err)
				}
			}

			// Stop the timer before cleanup.
			b.StopTimer()

			// Restore default config.
			config.EnableEndpointDiscovery = opdefault.DefaultEnableEndpointDiscovery
			config.EnableRelayToClusterIP = opdefault.DefaultEnableRelayToClusterIP
			config.DataplaneMode = config.NewDataplaneMode(opdefault.DefaultDataplaneMode)
		})
	}
}
