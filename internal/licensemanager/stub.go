package licensemanager

import (
	"context"

	"github.com/go-logr/logr"
	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
	licensecfg "github.com/l7mp/stunner/v2/pkg/config/license"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

type stubMgr struct{ opCh chan<- event.Event }

func NewStubManager(_ string, _ logr.Logger) Manager { return &stubMgr{} }

func (m *stubMgr) Start(ctx context.Context) error {
	if m.opCh != nil {
		// The status goes out through a latest-value sender, the same way the premium
		// manager pushes its updates: the operator channel has many producers and is not
		// drained at all on a standby replica, so a license status must neither block the
		// manager nor disturb what other producers put there.
		event.NewLatestSender(m.opCh).Start(ctx).
			Send(event.NewEventLicense(m.Status()))
	}
	<-ctx.Done()
	return nil
}
func (*stubMgr) NeedLeaderElection() bool                   { return false }
func (*stubMgr) Validate(_ licensecfg.Feature) bool         { return false }
func (*stubMgr) Status() stnrapiv1.LicenseStatus            { return stnrapiv1.NewEmptyLicenseStatus() }
func (*stubMgr) LastError() error                           { return nil }
func (m *stubMgr) SetOperatorChannel(ch chan<- event.Event) { m.opCh = ch }
func (*stubMgr) GenerateLicenseConfig() (stnrapiv1.LicenseConfig, error) {
	return stnrapiv1.LicenseConfig{}, nil
}
func (*stubMgr) SubscriptionType() licensecfg.SubscriptionType {
	return licensecfg.NewNilSubscriptionType()
}
