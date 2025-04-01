package event

import "sync"

type EventChannel interface {
	// Channel returns the event channel without incrementing the reference count.
	Channel() chan Event
	// Get returns the event channel and increments the reference count.
	Get() chan Event
	// Put decrements the reference count.
	Put()
	// Close blocks until all references are gone and then closes the channel.
	Close()
}

type eventChannel struct {
	ch chan Event
	wg sync.WaitGroup
}

func NewEventChannel(ch chan Event) EventChannel {
	return &eventChannel{ch: ch, wg: sync.WaitGroup{}}
}

func (ec *eventChannel) Channel() chan Event { return ec.ch }
func (ec *eventChannel) Get() chan Event     { ec.wg.Add(1); return ec.ch }
func (ec *eventChannel) Put()                { ec.wg.Done() }
func (ec *eventChannel) Close()              { ec.wg.Wait(); close(ec.ch) }
