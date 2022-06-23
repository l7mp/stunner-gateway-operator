package event

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type EventUpsert struct {
	Type   EventType
	Object client.Object
	// Params map[string]string
}

// NewEvent returns a new Upsert event
func NewEventUpsert(o client.Object) *EventUpsert {
	return &EventUpsert{Type: EventTypeUpsert, Object: o}
}

func (e *EventUpsert) GetType() EventType {
	return e.Type
}

func (e *EventUpsert) String() string {
	return fmt.Sprintf("%s: %s/%s", e.Type.String(),
		e.Object.GetName(), e.Object.GetNamespace())
}
