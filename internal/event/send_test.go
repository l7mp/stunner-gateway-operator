package event

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEvent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "event")
}

var _ = Describe("Send", func() {
	Context("When the context has ended", func() {
		It("should give up instead of blocking", func() {
			ch := make(chan Event) // unbuffered, nobody reads
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			Expect(Send(ctx, ch, NewEventReconcile("x"))).To(BeFalse(),
				"a cancelled context must not block the sender")
		})
	})

	Context("When the channel has room", func() {
		It("should deliver the event", func() {
			ch := make(chan Event, 1)
			Expect(Send(context.Background(), ch, NewEventReconcile("x"))).To(BeTrue())
			Expect((<-ch).(*EventReconcile).Sender).To(Equal("x"))
		})
	})
})

var _ = Describe("SendCoalesced", func() {
	Context("When the channel is full", func() {
		It("should drop the stale entries, keep the newest, and report both facts", func() {
			ch := make(chan Event, 2)
			for i := 0; i < 5; i++ {
				dropped, ok := SendCoalesced(context.Background(), ch, NewEventUpdate(i))
				Expect(ok).To(BeTrue())
				// the first two find room; from then on each send evicts exactly one,
				// the oldest, to make room for itself
				want := 0
				if i >= 2 {
					want = 1
				}
				Expect(dropped).To(Equal(want), "send %d", i)
			}
			Expect(ch).To(HaveLen(2))
			// the two newest survive, in order
			Expect((<-ch).(*EventUpdate).Generation).To(Equal(3))
			Expect((<-ch).(*EventUpdate).Generation).To(Equal(4))
		})
	})

	Context("When the context has ended and nothing can be dropped", func() {
		It("should report that it gave up, not merely that it dropped nothing", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			// unbuffered: the drain finds nothing, the send never succeeds
			dropped, ok := SendCoalesced(ctx, make(chan Event), NewEventUpdate(0))
			Expect(ok).To(BeFalse(), "the delivery failed")
			Expect(dropped).To(BeZero(), "and it is not the drop count that says so")
		})
	})
})

var _ = Describe("SendOrDrop", func() {
	Context("When the consumer has room", func() {
		It("should deliver and say so", func() {
			ch := make(chan Event, 1)
			Expect(SendOrDrop(ch, NewEventReconcile("x"))).To(BeTrue())
			Expect((<-ch).(*EventReconcile).Sender).To(Equal("x"))
		})
	})

	Context("When the consumer is behind", func() {
		It("should drop the new event rather than block, and leave the pending ones alone", func() {
			ch := make(chan Event, 1)
			Expect(SendOrDrop(ch, NewEventUpdate(1))).To(BeTrue())
			Expect(SendOrDrop(ch, NewEventUpdate(2))).To(BeFalse(), "no room, so it is dropped")
			Expect(ch).To(HaveLen(1))
			Expect((<-ch).(*EventUpdate).Generation).To(Equal(1),
				"the pending event survives: this is not coalescing")
		})
	})
})

var _ = Describe("LatestSender", func() {
	Context("When the consumer is reading", func() {
		It("should deliver what it was handed", func() {
			ch := make(chan Event, 1)
			sender := NewLatestSender(ch).Start(context.Background())
			sender.Send(NewEventReconcile("x"))
			var got Event
			Eventually(ch).Should(Receive(&got))
			Expect(got.(*EventReconcile).Sender).To(Equal("x"))
		})
	})

	Context("When the consumer is not reading", func() {
		It("should abandon a superseded event and deliver only the newest", func() {
			// one slot, already taken, so the sender cannot deliver anything yet
			ch := make(chan Event, 1)
			ch <- NewEventReconcile("blocker")

			sender := NewLatestSender(ch).Start(context.Background())
			sender.Send(NewEventUpdate(1))

			// The sleep is load-bearing: the sender has to pick 1 up and commit to it,
			// because abandoning is only possible for an event it is already blocked on.
			// Without this the register already holds 3 by the time it first reads, and
			// the spec passes even with the abandon arm deleted.
			time.Sleep(20 * time.Millisecond)

			sender.Send(NewEventUpdate(2))
			sender.Send(NewEventUpdate(3))

			Expect((<-ch).(*EventReconcile).Sender).To(Equal("blocker"))

			// Collect everything that comes out, rather than matching on content
			// inside Receive: Eventually would swallow a wrong value and retry until
			// a right one turned up, hiding exactly what this spec is here to catch.
			gens := []int{}
			Eventually(func() []int {
				select {
				case e := <-ch:
					gens = append(gens, e.(*EventUpdate).Generation)
				default:
				}
				return gens
			}).Should(ContainElement(3), "the newest event always gets through")
			Expect(gens).NotTo(ContainElement(1),
				"an event superseded while the sender was blocked on it is abandoned")
		})

		It("should not block the producers, however many there are", func() {
			ch := make(chan Event) // unbuffered, nobody reads
			sender := NewLatestSender(ch).Start(context.Background())

			// Four senders rather than one, because the hand-over has to hold up under
			// concurrent callers. This cannot fail reliably if the lock is removed, the
			// window is a few instructions wide, so read it as documentation of the
			// contract rather than as a guard against regressing it.
			done := make(chan struct{})
			go func() {
				defer close(done)
				var wg sync.WaitGroup
				for p := 0; p < 4; p++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for i := 0; i < 100; i++ {
							sender.Send(NewEventUpdate(i))
						}
					}()
				}
				wg.Wait()
			}()
			Eventually(done, time.Second).Should(BeClosed())
		})
	})

	Context("When the context ends", func() {
		It("should stop delivering, and still not block the producer", func() {
			ch := make(chan Event, 1)
			ctx, cancel := context.WithCancel(context.Background())
			sender := NewLatestSender(ch).Start(ctx)
			cancel()
			// the sender may be anywhere, so let it notice it is done
			Consistently(ch, 50*time.Millisecond).ShouldNot(Receive())

			// Nothing takes events out of the hand-over slot any more, so this is where
			// a hand-over that did not make room first would wedge forever. One event
			// would fit and prove nothing: it takes a second one to fill the slot.
			handedOver := make(chan struct{})
			go func() {
				defer close(handedOver)
				for i := 0; i < 3; i++ {
					sender.Send(NewEventUpdate(i))
				}
			}()
			Eventually(handedOver, time.Second).Should(BeClosed(),
				"Send must not block once the sender has stopped")

			Consistently(ch, 100*time.Millisecond).ShouldNot(Receive(),
				"a sender whose context ended delivers nothing")
		})
	})
})
