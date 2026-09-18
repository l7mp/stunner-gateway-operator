package event

import (
	"fmt"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
)

// EventLicense carries the licensing status from the license manager to the operator. The
// operator forwards it to the config discovery server and triggers a re-render, since the
// rendered dataplane config depends on the licensed feature set.
type EventLicense struct {
	Type   EventType
	Status stnrapiv1.LicenseStatus
}

// NewEventLicense creates a license status event.
func NewEventLicense(status stnrapiv1.LicenseStatus) *EventLicense {
	return &EventLicense{Type: EventTypeLicense, Status: status}
}

func (e *EventLicense) GetType() EventType {
	return e.Type
}

func (e *EventLicense) String() string {
	return fmt.Sprintf("%s: %s", e.Type.String(), e.Status.String())
}
