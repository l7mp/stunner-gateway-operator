package event

import (
	"context"
	"sync"
)

// Send delivers e on ch unless ctx ends first. It returns false when it gave up. Every send
// between the operator subsystems goes through one of the senders in this file, so that the
// semantics of a send are written down at the call site, and so that a consumer that has already
// stopped can never block a producer past its own context.
func Send(ctx context.Context, ch chan<- Event, e Event) bool {
	select {
	case ch <- e:
		return true
	case <-ctx.Done():
		return false
	}
}

// SendCoalesced delivers e on ch, dropping stale pending entries to make room whenever the channel
// is full, so a slow consumer only ever sees the newest events. It reports how many stale entries
// it dropped, and whether e itself was delivered: the two are independent, since a send can give up
// on ctx after having already dropped entries.
//
// The caller must be the channel's ONLY producer. The loop drops whatever sits at the head of the
// channel, which on a shared channel is somebody else's event, so coalescing there silently
// destroys another producer's work. This is why the channels that carry more than one producer,
// like the operator channel, are handed out as send-only: that makes the mistake a compile error.
func SendCoalesced(ctx context.Context, ch chan Event, e Event) (dropped int, ok bool) {
	for {
		select {
		case ch <- e:
			return dropped, true
		case <-ctx.Done():
			return dropped, false
		default:
		}
		select {
		case <-ch:
			dropped++
		default:
			// A concurrent reader emptied the channel between the failed send above
			// and this drain attempt; the outer loop will retry the send.
		}
	}
}

// SendOrDrop delivers e on ch if the consumer has room for it right now, and drops it otherwise.
// It reports whether the event was delivered.
func SendOrDrop(ch chan<- Event, e Event) bool {
	select {
	case ch <- e:
		return true
	default:
		return false
	}
}

// LatestSender sends the latest event on a possibly blocking channel. If the channel is available
// it performs a normal send, otherwise it drops the last event waiting to be sent and instead
// sends the new one.
type LatestSender struct {
	ch chan<- Event
	// box holds at most one event, the latest one.
	box      chan Event
	handOver sync.Mutex
}

func NewLatestSender(ch chan<- Event) *LatestSender {
	return &LatestSender{ch: ch, box: make(chan Event, 1)}
}

// Send hands e over, replacing an event that is still waiting. It never blocks.
func (s *LatestSender) Send(e Event) {
	s.handOver.Lock()
	defer s.handOver.Unlock()

	// make room: discard an event that has not been taken yet, if there is one
	select {
	case <-s.box:
	default:
	}

	s.box <- e
}

// Start puts the sender to work and returns it, so creating and starting one is a single
// expression. It returns immediately: the delivering happens in a goroutine of its own, which ends
// with ctx.
//
// It does not block, unlike the Start of the subsystems the manager runs. This is not a manager
// runnable; it belongs to whoever sends through it.
func (s *LatestSender) Start(ctx context.Context) *LatestSender {
	go s.run(ctx)
	return s
}

// run delivers events until ctx ends, blocking on the channel for as long as the consumer needs.
func (s *LatestSender) run(ctx context.Context) {
	for {
		var e Event
		select {
		case e = <-s.box:
		case <-ctx.Done():
			return
		}

		// Keep trying to deliver e, but give it up the moment a newer event arrives: the
		// consumer must never see an event that is already obsolete.
		for e != nil {
			select {
			case s.ch <- e:
				e = nil
			case newer := <-s.box:
				e = newer
			case <-ctx.Done():
				return
			}
		}
	}
}
