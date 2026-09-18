// Implementation of the license manager for unlocking enterprise features
package licensemanager

import (
	"context"

	"github.com/go-logr/logr"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
	licensecfg "github.com/l7mp/stunner/v2/pkg/config/license"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

var licenseManagerConstructor = NewStubManager

// Manager is a global license manager that encapsulates the license management logics. Runs on
// every operator replica, leader and standby, so the licensing status is known the moment a
// replica becomes leader.
type Manager interface {
	// Start runs the license manager until the context ends. The manager sends an
	// EventLicense to the operator channel at startup and on every status change.
	Start(context.Context) error
	// NeedLeaderElection reports false: the license manager runs on every replica.
	NeedLeaderElection() bool
	// Validate checks whether a client is entitled to use a feature.
	Validate(feature licensecfg.Feature) bool
	// SubscriptionType returns the current subscription type (e.g., free, member, enterprise).
	SubscriptionType() licensecfg.SubscriptionType
	// Generate a license configuration for the dataplane.
	GenerateLicenseConfig() (stnrapiv1.LicenseConfig, error)
	// SetOperatorChannel sets up the operator channel where the manager sends its license
	// status events. Send-only on purpose: the manager is one of many producers there, so it
	// must never coalesce that channel.
	SetOperatorChannel(c chan<- event.Event)
	// LastError returns the last license manager error.
	LastError() error
	// Status returns the current licensing status.
	Status() stnrapiv1.LicenseStatus
}

func NewManager(key string, logger logr.Logger) Manager {
	return licenseManagerConstructor(key, logger)
}
