package event

import (
	"fmt"
)

// render event

type EventRender struct {
	Type   EventType
	Reason string
	// Params map[string]string
}

// NewEvent returns an empty event
func NewEventRender(reason string) *EventRender {
	return &EventRender{Type: EventTypeRender, Reason: reason}
}

func (e *EventRender) GetType() EventType {
	return e.Type
}

func (e *EventRender) String() string {
	return fmt.Sprintf("%s: %s", e.Type.String(), e.Reason)
}
