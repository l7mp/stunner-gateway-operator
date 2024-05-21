package event

import "fmt"

// EventFinalize is an event that requests the renderer to invalidate managed resources.
type EventFinalize struct {
	Type       EventType
	Generation int
	// Reason string
	// Params map[string]string
}

// NewEvent returns an empty event
func NewEventFinalize(gen int) *EventFinalize {
	return &EventFinalize{Type: EventTypeFinalize, Generation: gen}
}

func (e *EventFinalize) GetType() EventType {
	return e.Type
}

func (e *EventFinalize) String() string {
	return fmt.Sprintf("%s: generation: %d", e.Type.String(), e.Generation)
}
