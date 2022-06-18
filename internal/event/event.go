package event

import (
	"fmt"
	// "sync"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/client"
	// // stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// // gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// EventType species the type of an event sent to the operator
type EventType int

const (
	// EventTypeRender indicates a request for operator to generate the STUNner configuration
	EventTypeRender EventType = iota + 1
	EventTypeUnknown
)

const (
	eventTypeRenderStr = "render-configuration"
)

// NewEventType parses an event type specification
func NewEventType(raw string) (EventType, error) {
	switch raw {
	case eventTypeRenderStr:
		return EventTypeRender, nil
	default:
		return EventTypeUnknown, fmt.Errorf("unknown event type: %q", raw)
	}
}

// String returns a string representation for the evententication mechanism
func (a EventType) String() string {
	switch a {
	case EventTypeRender:
		return eventTypeRenderStr
	default:
		return "<unknown>"
	}
}

// Event defines an event sent to the operator
type Event struct {
	Type   EventType
	Params map[string]string
}

// NewEvent returns an empty event
func NewEvent(t EventType) Event {
	return Event{Type: t, Params: make(map[string]string)}
}

func (e *Event) String() string {
	return e.Type.String()
}
