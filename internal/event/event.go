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

// String returns a string representation for the evententication mechanism
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

// Event defines an event sent to the operator
type Event interface {
	GetType() EventType
	String() string
}
