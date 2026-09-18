package operator

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/controllers"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

func TestOperator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "operator")
}

// pollInterval is how often the channel matchers look; short, because every wait in this file is
// a handful of milliseconds of real work.
const pollInterval = 2 * time.Millisecond

// noopController satisfies controllers.Controller with a name and a no-op reconcile, so the
// operator can be instantiated without a controller-runtime manager.
type noopController struct{ name controllers.ControllerName }

func (c *noopController) Name() controllers.ControllerName { return c.name }
func (c *noopController) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// newTestOperator builds a minimal operator wired to the provided stub channels: no manager, no
// real controllers, only the fields used by eventLoop. The named controllers populate the
// startup gate.
func newTestOperator(opCh chan event.Event, updaterCh, configCh, renderCh chan event.Event, names ...controllers.ControllerName) *Operator {
	o := &Operator{
		operatorCh: opCh,
		configCh:   configCh,
		updaterCh:  updaterCh,
		renderCh:   renderCh,
		tracker:    config.NewProgressTracker(),
		log:        logr.Discard(),
	}
	for _, n := range names {
		o.controllers = append(o.controllers, &noopController{name: n})
	}
	return o
}

func fastThrottle(throttle, gate time.Duration) {
	origThrottle, origGate := config.ThrottleTimeout, config.StartupRenderTimeout
	config.ThrottleTimeout, config.StartupRenderTimeout = throttle, gate
	DeferCleanup(func() { config.ThrottleTimeout, config.StartupRenderTimeout = origThrottle, origGate })
}

// runEventLoop starts the loop and stops it when the spec ends.
func runEventLoop(o *Operator) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)
	go o.eventLoop(ctx)
	return ctx
}

func licenseKnown() event.Event { return event.NewEventLicense(stnrapiv1.NewEmptyLicenseStatus()) }

func expectRender(renderCh chan event.Event, within time.Duration, msg string) {
	GinkgoHelper()
	var e event.Event
	Eventually(renderCh, within, pollInterval).Should(Receive(&e), msg)
	Expect(e.GetType()).To(Equal(event.EventTypeRender))
}

func expectNoRender(renderCh chan event.Event, within time.Duration, msg string) {
	GinkgoHelper()
	Consistently(renderCh, within, pollInterval).ShouldNot(Receive(), msg)
}

var _ = Describe("Event loop", func() {
	Context("When the config discovery consumer never drains its channel", func() {
		It("should keep delivering updates to the updater", func() {
			fastThrottle(10*time.Millisecond, time.Hour)

			// numUpdates must exceed configCh's buffer size so a blocking send would
			// deadlock.
			const numUpdates = 30

			// configCh is intentionally NEVER drained, simulating permanently
			// slow/absent stunnerd clients whose UpdateConfig() call takes longer than
			// ThrottleTimeout.
			configCh := make(chan event.Event, 10)
			updaterCh := make(chan event.Event, numUpdates+5)
			renderCh := make(chan event.Event, numUpdates+5)

			opCh := make(chan event.Event, channelBufferSize)
			runEventLoop(newTestOperator(opCh, updaterCh, configCh, renderCh))

			for i := 0; i < numUpdates; i++ {
				opCh <- event.NewEventUpdate(i)
			}

			for i := 0; i < numUpdates; i++ {
				Eventually(updaterCh, 3*time.Second, pollInterval).Should(Receive(),
					"deadlock: only %d/%d updates reached updaterCh (configCh cap=%d)",
					i, numUpdates, cap(configCh))
			}

			Expect(len(configCh)).To(BeNumerically("<=", cap(configCh)),
				"configCh should not exceed its capacity")
			Expect(updaterCh).To(BeEmpty(), "updaterCh should be fully drained")
		})

		It("should still process reconcile events and render", func() {
			fastThrottle(10*time.Millisecond, time.Hour)

			configCh := make(chan event.Event, 10) // never drained
			updaterCh := make(chan event.Event, 50)
			renderCh := make(chan event.Event, 50)

			opCh := make(chan event.Event, channelBufferSize)
			ctx := runEventLoop(newTestOperator(opCh, updaterCh, configCh, renderCh))

			// Saturate configCh with updates.
			const numUpdates = 20
			for i := 0; i < numUpdates; i++ {
				opCh <- event.NewEventUpdate(i)
			}

			// Drain updaterCh in the background so it never becomes a secondary blocker.
			go func() {
				defer GinkgoRecover()
				for {
					select {
					case <-updaterCh:
					case <-ctx.Done():
						return
					}
				}
			}()

			// No controllers, so only the license status gates the first render.
			opCh <- licenseKnown()
			opCh <- event.NewEventReconcile("test")
			expectRender(renderCh, 3*time.Second,
				"EventTypeReconcile not processed with saturated configCh")
		})
	})
})

var _ = Describe("Startup gate", func() {
	var opCh, updaterCh, configCh, renderCh chan event.Event

	BeforeEach(func() {
		opCh = make(chan event.Event, channelBufferSize)
		updaterCh, configCh = make(chan event.Event, 10), make(chan event.Event, 10)
		renderCh = make(chan event.Event, 10)
	})

	Context("When controllers are registered", func() {
		It("should hold the render until every one of them reported", func() {
			fastThrottle(10*time.Millisecond, time.Hour)
			runEventLoop(newTestOperator(opCh, updaterCh, configCh, renderCh, "a", "b", "c"))

			opCh <- licenseKnown()
			opCh <- event.NewEventReconcile("a")
			opCh <- event.NewEventReconcile("b")
			opCh <- event.NewEventReconcile("a") // a repeat does not count for c
			expectNoRender(renderCh, 100*time.Millisecond, "rendered before controller c reported")

			opCh <- event.NewEventReconcile("c")
			expectRender(renderCh, time.Second,
				"the last controller's report must open the gate and render")
		})

		It("should throttle normally once the gate is open", func() {
			fastThrottle(10*time.Millisecond, time.Hour)
			runEventLoop(newTestOperator(opCh, updaterCh, configCh, renderCh, "a"))

			opCh <- licenseKnown()
			opCh <- event.NewEventReconcile("a")
			expectRender(renderCh, time.Second, "the gate must open and render")

			for i := 0; i < 5; i++ {
				opCh <- event.NewEventReconcile("a")
			}
			expectRender(renderCh, time.Second, "post-gate reconcile must render")
			expectNoRender(renderCh, 50*time.Millisecond,
				"a burst within the throttle window must coalesce")
		})
	})

	Context("When the license status is not known yet", func() {
		It("should hold the render until it arrives, and forward it to config discovery", func() {
			fastThrottle(10*time.Millisecond, time.Hour)
			runEventLoop(newTestOperator(opCh, updaterCh, configCh, renderCh, "a"))

			opCh <- event.NewEventReconcile("a")
			expectNoRender(renderCh, 100*time.Millisecond,
				"rendered before the license status was known")

			opCh <- licenseKnown()
			expectRender(renderCh, time.Second,
				"the license status must open the gate and render")
			Expect(configCh).To(HaveLen(1),
				"the license event is forwarded to the config discovery server")
		})
	})

	Context("When a controller never reports", func() {
		It("should open the gate on the startup timeout", func() {
			fastThrottle(10*time.Millisecond, 100*time.Millisecond)
			runEventLoop(newTestOperator(opCh, updaterCh, configCh, renderCh, "a", "never"))

			opCh <- licenseKnown()
			opCh <- event.NewEventReconcile("a")
			expectNoRender(renderCh, 50*time.Millisecond, "rendered before the gate timed out")
			expectRender(renderCh, time.Second, "the gate timeout must render")
		})
	})
})
