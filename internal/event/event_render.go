package event

import (
	"fmt"
)

// render event

type EventRender struct {
	Type   EventType
	Origin string
	Reason string
	// Params map[string]string
}

// NewEvent returns an empty event
func NewEventRender(origin, reason string) *EventRender {
	return &EventRender{Type: EventTypeRender, Origin: origin, Reason: reason}
}

func (e *EventRender) GetType() EventType {
	return e.Type
}

func (e *EventRender) String() string {
	return fmt.Sprintf("%s: %s: %s", e.Type.String(), e.Origin, e.Reason)
}
