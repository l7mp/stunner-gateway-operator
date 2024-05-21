package event

// reconcile event
type EventReconcile struct {
	Type EventType
	// Reason string
	// Params map[string]string
}

// NewEvent returns an empty event
func NewEventReconcile() *EventReconcile {
	return &EventReconcile{Type: EventTypeReconcile}
}

func (e *EventReconcile) GetType() EventType {
	return e.Type
}

func (e *EventReconcile) String() string {
	return e.Type.String()
}
