// Implementation of the license manager for unlocking enterprise features
package licensemanager

import (
	"context"

	"github.com/go-logr/logr"

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	licensecfg "github.com/l7mp/stunner/pkg/config/license"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

var licenseManagerConstructor = NewStubManager

// Manager is a global license manager that encapsulates the license management logics
type Manager interface {
	// Start runs the license manager.
	Start(context.Context) error
	// Validate checks whether a client is entitled to use a feature.
	Validate(feature licensecfg.Feature) bool
	// SubscriptionType returns the current subscription type (e.g., free, member, enterprise).
	SubscriptionType() string
	// Generate a license configuration for the dataplane.
	GenerateLicenseConfig() (stnrv1.LicenseConfig, error)
	// SetOperatorChannel sets up the operator channel where the manager can send rendering
	SetOperatorChannel(c chan event.Event)
	// LastError returns the last license manager error.
	LastError() error
}

type Config struct {
	CustomerKey          string
	LicenseManagerClient any
	Logger               logr.Logger
}

func NewManager(config Config) Manager {
	return licenseManagerConstructor(config)
}
