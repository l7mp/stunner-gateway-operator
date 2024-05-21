package event

import (
	"fmt"
)

// EventType specifies the type of an event sent by the threads of the operator.
type EventType int

const (
	EventTypeUnknown EventType = iota + 1
	// EventTypeRender is sent by the operator to the renderer to request rendering.
	EventTypeRender
	// EventTypeReconcile is sent by the controllers to the operator to trigger reconciliation.
	EventTypeReconcile
	// EventTypeUpdate is created by the renderer thread holding a set of Kubernetes resources
	// that must be crater, updated or deleted. The event is sent from the renderer to the
	// operator, which passes the event on the updater thread for processing.
	EventTypeUpdate
	// EventTypeFinalize is sent from the operator to the renderer to commence the finalization
	// cycle.
	EventTypeFinalize
	// EventTypeFinalize is used by the updater to acknowledge that it has finished processing
	// an update generation.
	EventTypeAck
)

const (
	eventTypeRenderStr      = "render"
	eventTypeReconcileStr   = "reconcile"
	eventTypeUpdateStr      = "update"
	eventTypeFinalizeStr    = "finalize"
	eventTypeAckResponseStr = "acknowledgement"
)

// NewEventType parses an event type specification
func NewEventType(raw string) (EventType, error) {
	switch raw {
	case eventTypeRenderStr:
		return EventTypeRender, nil
	case eventTypeReconcileStr:
		return EventTypeReconcile, nil
	case eventTypeUpdateStr:
		return EventTypeUpdate, nil
	case eventTypeAckResponseStr:
		return EventTypeAck, nil
	default:
		return EventTypeUnknown, fmt.Errorf("unknown event type: %q", raw)
	}
}

// String returns a string representation for an event
func (a EventType) String() string {
	switch a {
	case EventTypeRender:
		return eventTypeRenderStr
	case EventTypeReconcile:
		return eventTypeReconcileStr
	case EventTypeUpdate:
		return eventTypeUpdateStr
	case EventTypeFinalize:
		return eventTypeFinalizeStr
	case EventTypeAck:
		return eventTypeAckResponseStr
	default:
		return "<unknown>"
	}
}

// Event defines an event sent to/from the operator
type Event interface {
	GetType() EventType
	String() string
}
