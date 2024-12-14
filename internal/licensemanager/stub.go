package licensemanager

import (
	"context"

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	licensecfg "github.com/l7mp/stunner/pkg/config/license"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

// license manager stub
type stubMgr struct{}

func NewStubManager(_ Config) Manager { return &stubMgr{} }

func (_ *stubMgr) Start(_ context.Context) error         { return nil }
func (_ *stubMgr) Validate(_ licensecfg.Feature) bool    { return true }
func (_ *stubMgr) SubscriptionType() string              { return "free" }
func (_ *stubMgr) LastError() error                      { return nil }
func (_ *stubMgr) SetOperatorChannel(_ chan event.Event) {}
func (_ *stubMgr) GenerateLicenseConfig() (stnrv1.LicenseConfig, error) {
	return stnrv1.LicenseConfig{}, nil
}
