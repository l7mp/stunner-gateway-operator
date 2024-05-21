package event

import "fmt"

// render event
type EventRender struct {
	Type       EventType
	Generation int
	// Reason string
	// Params map[string]string
}

// NewEvent returns an empty event
func NewEventRender(gen int) *EventRender {
	return &EventRender{Type: EventTypeRender, Generation: gen}
}

func (e *EventRender) GetType() EventType {
	return e.Type
}

func (e *EventRender) String() string {
	return fmt.Sprintf("%s: generation: %d", e.Type.String(), e.Generation)
}
