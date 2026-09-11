package renderer

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	licensemgr "github.com/l7mp/stunner-gateway-operator/internal/licensemanager"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/stretchr/testify/require"
)

func TestEmptyManagedSnapshot(t *testing.T) {
	previous := store.GatewayClasses
	store.GatewayClasses = store.NewGatewayClassStore()
	defer func() { store.GatewayClasses = previous }()
	for _, enabled := range []bool{false, true} {
		r := NewDefaultRenderer(RendererConfig{PublishEmptyConfig: enabled, Logger: logr.Discard(), LicenseManager: licensemgr.NewManager("", logr.Discard())}).(*renderer)
		ch := make(chan event.Event, 1)
		r.operatorCh = event.NewEventChannel(ch)
		r.gen = 7
		r.renderManagedGateways(event.NewEventRender(7))
		if !enabled {
			require.Empty(t, ch)
			continue
		}
		select {
		case e := <-ch:
			u := e.(*event.EventUpdate)
			require.Equal(t, 7, u.Generation)
			require.Empty(t, u.ConfigQueue)
			require.True(t, u.RequestAck)
		default:
			t.Fatal("empty HA snapshot did not reach CDS")
		}
	}
}
