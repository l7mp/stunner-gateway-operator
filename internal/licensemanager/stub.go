package licensemanager

import (
	"context"

	"github.com/go-logr/logr"
	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	licensecfg "github.com/l7mp/stunner/pkg/config/license"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// license manager stub
type stubMgr struct{}

func NewStubManager(_ string, _ logr.Logger) Manager { return &stubMgr{} }

func (_ *stubMgr) Start(_ context.Context) error           { return nil }
func (_ *stubMgr) Validate(_ licensecfg.Feature) bool      { return true }
func (_ *stubMgr) Status() stnrv1.LicenseStatus            { return stnrv1.NewEmptyLicenseStatus() }
func (_ *stubMgr) LastError() error                        { return nil }
func (_ *stubMgr) SetOperatorChannel(_ event.EventChannel) {}
func (_ *stubMgr) GenerateLicenseConfig() (stnrv1.LicenseConfig, error) {
	return stnrv1.LicenseConfig{}, nil
}
func (_ *stubMgr) SubscriptionType() licensecfg.SubscriptionType {
	return licensecfg.NewNilSubscriptionType()
}
