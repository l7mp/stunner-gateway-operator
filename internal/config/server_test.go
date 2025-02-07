package config

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	cdsclient "github.com/l7mp/stunner/pkg/config/client"
	"github.com/l7mp/stunner/pkg/logger"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// var testerLogLevel = zapcore.Level(-4)
// var testerLogLevel = zapcore.DebugLevel
var testerLogLevel = zapcore.ErrorLevel

// const stunnerLogLevel = "all:TRACE"
const stunnerLogLevel = "all:ERROR"

// Steps:
// - starting CDS server
// - creating CDS client instance for testing Load()
// - creating CDS client instance 1 for testing Watch()
// - creating a config for the loader
// - creating a config for the 1st watcher
// - updating the config of the loader and the 1st watcher with unchanged configs
// - updating the config of the 1st watcher with a new config
// - creating CDS client instance for a 2nd watcher
// - adding a config CDS for the 2nd watcher
// - removing the config for the 1st watcher
// - closing the 2nd watcher
// - reinstalling the 2nd watcher
// - closing the connection of the 2nd watcher
// - removing all configs

func TestConfigDiscovery(t *testing.T) {
	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(testerLogLevel)
	z, err := zc.Build()
	assert.NoError(t, err, "logger created")
	zlogger := zapr.NewLogger(z)
	log := zlogger.WithName("tester")

	// setup a fast pinger so that we get a timely error notification
	cdsclient.PingPeriod = 200 * time.Millisecond
	cdsclient.PongWait = 300 * time.Millisecond
	cdsclient.WriteWait = 400 * time.Millisecond
	cdsclient.RetryPeriod = 400 * time.Millisecond

	testCDSAddr := getRandCDSAddr()
	log.Info("create server", "address", testCDSAddr)
	srv := NewCDSServer(testCDSAddr, zlogger)
	assert.NotNil(t, srv, "server")

	log.Info("starting CDS server")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, srv.Start(ctx), "cds server start")
	ch := srv.GetConfigUpdateChannel()

	time.Sleep(50 * time.Millisecond)
	logger := logger.NewLoggerFactory(stunnerLogLevel)

	id1 := "ns/gw1"
	addr1 := "http://" + testCDSAddr
	log.Info("creating CDS client instance 1", "address", addr1, "id", id1)
	cdsc1, err := cdsclient.New(addr1, id1, logger)
	assert.NoError(t, err, "cds client setup")

	id2 := "ns/gw2"
	addr2 := "http://" + testCDSAddr
	log.Info("creating CDS client instance 2", "address", addr2, "id", id2)
	cdsc2, err := cdsclient.New(addr2, id2, logger)
	assert.NoError(t, err, "cds client setup")

	ch1 := make(chan *stnrv1.StunnerConfig, 10)
	defer close(ch1)
	ch2 := make(chan *stnrv1.StunnerConfig, 10)
	defer close(ch2)
	err = cdsc1.Watch(ctx, ch1, true)
	assert.NoError(t, err, "watcher setup 1")
	err = cdsc2.Watch(ctx, ch2, true)
	assert.NoError(t, err, "watcher setup 2")

	time.Sleep(50 * time.Millisecond)

	// we should now have 2 client connections
	conns := srv.GetConnTrack()
	assert.NotNil(t, conns)
	snapshot := conns.Snapshot()
	assert.Len(t, snapshot, 2)

	// loading empty client config errs
	_, err = cdsc1.Load()
	assert.Error(t, err, "loading empty client config errs")
	_, err = cdsc2.Load()
	assert.Error(t, err, "loading empty client config errs")

	// we shouldn't have received any config updates
	c1 := watchConfig(ch1, 500*time.Millisecond)
	assert.Nil(t, c1)
	c2 := watchConfig(ch1, 500*time.Millisecond)
	assert.Nil(t, c2)

	log.Info("creating a config for the loader", "id", "ns/gw1")
	c1Ok := zeroConfig("ns", "gw1", "realm1")
	e := event.NewEventUpdate(0)
	e.ConfigQueue = []*stnrv1.StunnerConfig{c1Ok}
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c1)
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	c2, err = cdsc2.Load()
	assert.Error(t, err, "load")
	assert.Nil(t, c2)

	// we should have received a config update
	c1 = watchConfig(ch1, 1500*time.Millisecond)
	assert.NotNil(t, c1)
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	// no config update from client 2
	c2 = watchConfig(ch2, 150*time.Millisecond)
	assert.Nil(t, c2)

	// we should have a single config in the store
	cStore := srv.GetConfigStore()
	assert.Equal(t, 1, len(cStore.Snapshot()))
	c1s := cStore.Get("ns/gw1")
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")

	// license status client should return a nil license status
	lc, err := cdsclient.NewLicenseStatusClient(addr1, logger.NewLogger("license-status"))
	assert.NoError(t, err, "license client setup")
	status, err := lc.LicenseStatus(ctx)
	assert.NoError(t, err, "loading status 1 ok")
	assert.Equal(t, stnrv1.NewEmptyLicenseStatus(), status, "license 1 ok")

	log.Info("creating a config for the 2nd client", "id", "ns/gw2")
	c2Ok := zeroConfig("ns", "gw2", "realm2")
	e = event.NewEventUpdate(0)
	e.ConfigQueue = []*stnrv1.StunnerConfig{c1Ok, c2Ok}
	licenseStatus := stnrv1.LicenseStatus{
		EnabledFeatures:  []string{"a", "b", "c"},
		SubscriptionType: "test",
		LastUpdated:      "never",
		LastError:        "",
	}
	e.LicenseStatus = licenseStatus
	ch <- e

	time.Sleep(50 * time.Millisecond)

	// license status client should return the new license status
	status, err = lc.LicenseStatus(ctx)
	assert.NoError(t, err, "loading status 1 ok")
	assert.Equal(t, licenseStatus, status, "license 1 ok")

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client 1 config ok")
	assert.NotNil(t, c1)
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")
	c2, err = cdsc2.Load()
	assert.NoError(t, err, "loading client 2 config ok")
	assert.NotNil(t, c2)
	assert.True(t, c2Ok.DeepEqual(c2), "config ok")

	c1 = watchConfig(ch1, 150*time.Millisecond)
	assert.Nil(t, c1)
	c2 = watchConfig(ch2, 1500*time.Millisecond)
	assert.NotNil(t, c2)
	assert.True(t, c2Ok.DeepEqual(c2), "config ok")

	// we should have 2 configs in the store
	cStore = srv.GetConfigStore()
	assert.Equal(t, 2, len(cStore.Snapshot()))
	c1s = cStore.Get("ns/gw1")
	assert.NotNil(t, c1s)
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")
	c2s := cStore.Get("ns/gw2")
	assert.NotNil(t, c2s)
	assert.True(t, c2s.DeepEqual(c2Ok), "config ok")

	log.Info("updating the 2nd config", "id2", c2Ok.Admin.Name)
	c2Ok = zeroConfig("ns", "gw2", "realm2-new")
	e = event.NewEventUpdate(0)
	e.ConfigQueue = []*stnrv1.StunnerConfig{c1Ok, c2Ok}
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c1)
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")
	c2, err = cdsc2.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c2)
	assert.True(t, c2Ok.DeepEqual(c2), "config ok")

	c1 = watchConfig(ch1, 150*time.Millisecond)
	assert.Nil(t, c1)
	c2 = watchConfig(ch2, 1500*time.Millisecond)
	assert.NotNil(t, c2)
	assert.True(t, c2Ok.DeepEqual(c2), "config ok")

	// watcher3
	id3 := "ns/gw3"
	log.Info("creating CDS client instance 3", "address", addr2, "id", id3)
	cdsc3, err := cdsclient.New(addr2, id3, logger)
	assert.NoError(t, err, "cds client setup")

	ch3 := make(chan *stnrv1.StunnerConfig, 10)
	defer close(ch3)
	ctx2, cancel2 := context.WithCancel(context.Background())
	err = cdsc3.Watch(ctx2, ch3, false)
	assert.NoError(t, err, "watcher setup")

	time.Sleep(50 * time.Millisecond)

	// we should now have 3 client connections: store IDs for later use
	conns = srv.GetConnTrack()
	assert.NotNil(t, conns)
	snapshot = conns.Snapshot()
	assert.Len(t, snapshot, 3)
	connIds := []string{}
	for _, conn := range snapshot {
		connIds = append(connIds, conn.Id())
	}

	c1 = watchConfig(ch1, 1500*time.Millisecond)
	assert.Nil(t, c1)
	c2 = watchConfig(ch2, 150*time.Millisecond)
	assert.Nil(t, c2)
	c3 := watchConfig(ch3, 150*time.Millisecond)
	assert.Nil(t, c3)

	log.Info("adding a config CDS for the 3rd client", "id", "ns/gw3")
	c3Ok := zeroConfig("ns", "gw3", "realm3_new")
	e = event.NewEventUpdate(0)
	e.ConfigQueue = []*stnrv1.StunnerConfig{c1Ok, c2Ok, c3Ok}
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c1)
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")
	c2, err = cdsc2.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c2)
	assert.True(t, c2Ok.DeepEqual(c2), "config ok")
	c3, err = cdsc3.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c3)
	assert.True(t, c3Ok.DeepEqual(c3), "config ok")

	// watcher1 shouldn't receive an update
	c1 = watchConfig(ch1, 1500*time.Millisecond)
	assert.Nil(t, c1)
	c1 = watchConfig(ch1, 1500*time.Millisecond)
	assert.Nil(t, c1)
	c3 = watchConfig(ch3, 150*time.Millisecond)
	assert.NotNil(t, c3)
	assert.True(t, c3Ok.DeepEqual(c3), "config ok")

	// we should have 3 configs in the store
	cStore = srv.GetConfigStore()
	assert.Equal(t, 3, len(cStore.Snapshot()))
	c1s = cStore.Get("ns/gw1")
	assert.NotNil(t, c1s)
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")
	c2s = cStore.Get("ns/gw2")
	assert.NotNil(t, c2s)
	assert.True(t, c2s.DeepEqual(c2Ok), "config ok")
	c3s := cStore.Get("ns/gw3")
	assert.NotNil(t, c3s)
	assert.True(t, c3s.DeepEqual(c3Ok), "config ok")

	log.Info("removing the config for the 2nd client", "id", "ns/gw2")
	e = event.NewEventUpdate(0)
	e.ConfigQueue = []*stnrv1.StunnerConfig{c1Ok, c3Ok}
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c1)
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")
	c2, err = cdsc2.Load()
	assert.Error(t, err, "load")
	assert.Nil(t, c2)
	c3, err = cdsc3.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.NotNil(t, c3)
	assert.True(t, c3Ok.DeepEqual(c3), "config ok")

	// watcher2 should have received nothing (deleted configs are not updated)
	c1 = watchConfig(ch1, 150*time.Millisecond)
	assert.Nil(t, c1)
	c2 = watchConfig(ch2, 150*time.Millisecond)
	assert.Nil(t, c2)
	c3 = watchConfig(ch3, 150*time.Millisecond)
	assert.Nil(t, c3)

	// we should have 2 configs in the store
	cStore = srv.GetConfigStore()
	assert.Equal(t, 2, len(cStore.Snapshot()))
	c1s = cStore.Get("ns/gw1")
	assert.NotNil(t, c1s)
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")
	c3s = cStore.Get("ns/gw3")
	assert.NotNil(t, c3s)
	assert.True(t, c3s.DeepEqual(c3Ok), "config ok")

	log.Info("closing the 3rd watcher", "id", "nw/gw3")
	cancel2()
	time.Sleep(50 * time.Millisecond)

	log.Info("reinstalling the 2nd watcher", "id", "nw/gw3")
	ch3 = make(chan *stnrv1.StunnerConfig, 10)
	defer close(ch3)
	ctx2, cancel2 = context.WithCancel(context.Background())
	defer cancel2()
	err = cdsc3.Watch(ctx2, ch3, false)
	assert.NoError(t, err, "watcher setup")
	time.Sleep(50 * time.Millisecond)

	// we should have received a valid config
	c3 = watchConfig(ch3, 1500*time.Millisecond)
	assert.NotNil(t, c3)
	assert.True(t, c3.DeepEqual(c3Ok), "config ok")

	log.Info("closing the connection of the 3rd watcher", "id", "nw/gw3")
	conns = srv.GetConnTrack()
	assert.NotNil(t, conns)
	snapshot = conns.Snapshot()
	// kill the connection(s) we do not remember
	for _, conn := range snapshot {
		if conn.Id() != connIds[0] && conn.Id() != connIds[1] {
			srv.RemoveClient(conn.Id())
		}
	}

	// after 2 pong-waits, clients should have reconnected
	time.Sleep(cdsclient.RetryPeriod)
	time.Sleep(cdsclient.RetryPeriod)

	// 3rd watcher should receive its config
	c3 = watchConfig(ch3, 150*time.Millisecond)
	assert.NotNil(t, c3)
	assert.True(t, c3.DeepEqual(c3Ok), "config ok")

	log.Info("removing all configs")
	e = event.NewEventUpdate(0)
	e.ConfigQueue = []*stnrv1.StunnerConfig{}
	ch <- e

	time.Sleep(50 * time.Millisecond)

	_, err = cdsc1.Load()
	assert.Error(t, err, "loading client config errs")

	// watcher2 should have received nothing
	c2 = watchConfig(ch2, 150*time.Millisecond)
	assert.Nil(t, c2)

	// we should have no configs in the store
	cStore = srv.GetConfigStore()
	assert.Equal(t, 0, len(cStore.Snapshot()))
}

func zeroConfig(namespace, name, realm string) *stnrv1.StunnerConfig {
	id := fmt.Sprintf("%s/%s", namespace, name)
	c := cdsclient.ZeroConfig(id)
	c.Auth.Realm = realm
	_ = c.Validate()
	return c
}

//nolint:unused
func packConfig(c *stnrv1.StunnerConfig) *corev1.ConfigMap {
	nsName := store.GetNameFromKey(c.Admin.Name)

	sc, _ := json.Marshal(c)
	s := string(sc)

	immutable := true
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nsName.Name,
			Namespace: nsName.Namespace,
			Labels: map[string]string{
				opdefault.OwnedByLabelKey: opdefault.OwnedByLabelValue,
			},
			Annotations: map[string]string{
				opdefault.RelatedGatewayKey: "dummy",
			},
		},
		Immutable: &immutable,
		Data: map[string]string{
			opdefault.DefaultStunnerdConfigfileName: s,
		},
	}
}

// wait for some configurable time for a watch element
func watchConfig(ch chan *stnrv1.StunnerConfig, d time.Duration) *stnrv1.StunnerConfig {
	select {
	case c := <-ch:
		// fmt.Println("++++++++++++ got config ++++++++++++: ", c.String())
		return c
	case <-time.After(d):
		// fmt.Println("++++++++++++ timeout ++++++++++++")
		return nil
	}
}

// run on random port
func getRandCDSAddr() string {
	rndPort := rand.Intn(10000) + 50000
	return fmt.Sprintf(":%d", rndPort)
}
