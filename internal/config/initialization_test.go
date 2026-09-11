package config

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryInitializationDoesNotOpenListener(t *testing.T) {
	addr := getRandCDSAddr()
	c := NewCDSServer(addr, logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.StartUpdates(ctx)
	select {
	case <-c.Initialized():
		t.Fatal("empty process claimed initialized")
	default:
	}
	e := event.NewEventUpdate(1)
	e.ConfigQueue = []*stnrv1.StunnerConfig{zeroConfig("ns", "gateway", "test")}
	c.GetConfigUpdateChannel() <- e
	select {
	case <-c.Initialized():
	case <-time.After(time.Second):
		t.Fatal("successful snapshot was not acknowledged")
	}
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if conn != nil {
		conn.Close()
	}
	require.Error(t, err, "initialization must not expose an unelected server")
	require.NoError(t, c.Server.Start(ctx))
	conn, err = net.DialTimeout("tcp", addr, time.Second)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestDiscoveryEmptySnapshotCanInitialize(t *testing.T) {
	c := NewCDSServer(getRandCDSAddr(), logr.Discard())
	require.NoError(t, c.ProcessUpdate(event.NewEventUpdate(1)))
	select {
	case <-c.Initialized():
	default:
		t.Fatal("a successfully rendered empty cluster never initialized")
	}
}
