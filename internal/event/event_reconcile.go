package event

import "fmt"

// EventReconcile is sent by a controller to the operator after it has refreshed its stores, to
// request a new rendering round. Sender names the controller, which the operator uses to tell
// when every controller has reported at least once after startup.
type EventReconcile struct {
	Type   EventType
	Sender string
}

// NewEventReconcile creates a reconcile request on behalf of the named controller.
func NewEventReconcile(sender string) *EventReconcile {
	return &EventReconcile{Type: EventTypeReconcile, Sender: sender}
}

func (e *EventReconcile) GetType() EventType {
	return e.Type
}

func (e *EventReconcile) String() string {
	return fmt.Sprintf("%s: sender: %s", e.Type.String(), e.Sender)
}
