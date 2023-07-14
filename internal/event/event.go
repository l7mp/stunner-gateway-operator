package event

import (
	"fmt"
)

// EventType specifies the type of an event sent to the operator
type EventType int

const (
	EventTypeRender EventType = iota + 1
	EventTypeUpsert
	EventTypeDelete
	EventTypeUpdate
	EventTypeUnknown
)

const (
	eventTypeRenderStr = "render"
	eventTypeUpsertStr = "upsert"
	eventTypeDeleteStr = "delete"
	eventTypeUpdateStr = "update"
)

// NewEventType parses an event type specification
func NewEventType(raw string) (EventType, error) {
	switch raw {
	case eventTypeRenderStr:
		return EventTypeRender, nil
	case eventTypeUpsertStr:
		return EventTypeUpsert, nil
	case eventTypeDeleteStr:
		return EventTypeDelete, nil
	case eventTypeUpdateStr:
		return EventTypeUpdate, nil
	default:
		return EventTypeUnknown, fmt.Errorf("unknown event type: %q", raw)
	}
}

// String returns a string representation for an event
func (a EventType) String() string {
	switch a {
	case EventTypeRender:
		return eventTypeRenderStr
	case EventTypeUpsert:
		return eventTypeUpsertStr
	case EventTypeDelete:
		return eventTypeDeleteStr
	case EventTypeUpdate:
		return eventTypeUpdateStr
	default:
		return "<unknown>"
	}
}

// Event defines an event sent to/from the operator
type Event interface {
	GetType() EventType
	String() string
}
