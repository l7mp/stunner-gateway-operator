package event

import "fmt"

// EventAck is used by the updater to acknowledge that the update has finished.
type EventAck struct {
	Type       EventType
	Generation int
	// Reason string
	// Params map[string]string
}

// NewEventAcknowledgement returns an empty event.
func NewEventAck(gen int) *EventAck {
	return &EventAck{Type: EventTypeAck, Generation: gen}
}

func (e *EventAck) GetType() EventType {
	return e.Type
}

func (e *EventAck) String() string {
	return fmt.Sprintf("%s: generation: %d", e.Type.String(), e.Generation)
}
