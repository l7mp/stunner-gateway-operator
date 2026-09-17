package licensemanager

import (
	"context"

	"github.com/go-logr/logr"
	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
	licensecfg "github.com/l7mp/stunner/v2/pkg/config/license"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

type stubMgr struct{}

func NewStubManager(_ string, _ logr.Logger) Manager { return &stubMgr{} }

func (*stubMgr) Start(_ context.Context) error           { return nil }
func (*stubMgr) Validate(_ licensecfg.Feature) bool      { return false }
func (*stubMgr) Status() stnrapiv1.LicenseStatus         { return stnrapiv1.NewEmptyLicenseStatus() }
func (*stubMgr) LastError() error                        { return nil }
func (*stubMgr) SetOperatorChannel(_ event.EventChannel) {}
func (*stubMgr) GenerateLicenseConfig() (stnrapiv1.LicenseConfig, error) {
	return stnrapiv1.LicenseConfig{}, nil
}
func (*stubMgr) SubscriptionType() licensecfg.SubscriptionType {
	return licensecfg.NewNilSubscriptionType()
}
