package event

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/client"
)

// EventType specifies the Kind of an object under deletion
type EventKind int

const (
	EventKindGatewayClass EventKind = iota + 1
	EventKindGatewayConfig
	EventKindGateway
	EventKindUDPRoute
	EventKindService
	EventKindUnknown
)

// String returns a string representation for an event
func (a EventKind) String() string {
	switch a {
	case EventKindGatewayClass:
		return "GatewayClass"
	case EventKindGatewayConfig:
		return "GatewayConfig"
	case EventKindGateway:
		return "EventKindGateway"
	case EventKindUDPRoute:
		return "UDPRoute"
	case EventKindService:
		return "Service"
	default:
		return "<unknown>"
	}
}

type EventDelete struct {
	Type EventType
	Kind EventKind
	Key  types.NamespacedName
}

// NewEventDelete returns a Delete event
func NewEventDelete(kind EventKind, key types.NamespacedName) *EventDelete {
	return &EventDelete{Type: EventTypeDelete, Kind: kind, Key: key}
}

func (e *EventDelete) GetType() EventType {
	return e.Type
}

func (e *EventDelete) String() string {
	return fmt.Sprintf("%s: %s of type %s", e.Type.String(), e.Key, e.Kind.String())
}
