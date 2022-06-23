package event

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventDelete struct {
	Type   EventType
	Object client.Object
	// Params map[string]string
}

// NewEventDelelet returns an Delete event
func NewEventDelete(o client.Object) *EventDelete {
	return &EventDelete{Type: EventTypeUpsert, Object: o}
}

func (e *EventDelete) GetType() EventType {
	return e.Type
}

func (e *EventDelete) String() string {
	return fmt.Sprintf("%s: %s/%s", e.Type.String(),
		e.Object.GetName(), e.Object.GetNamespace())
}
