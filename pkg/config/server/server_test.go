package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
	cdsclient "github.com/l7mp/stunner/pkg/config/client"
	"github.com/l7mp/stunner/pkg/logger"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

var testerLogLevel = zapcore.Level(-4)

//var testerLogLevel = zapcore.DebugLevel
//var testerLogLevel = zapcore.ErrorLevel

const stunnerLogLevel = "all:TRACE"

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

	cds := NewConfigDiscoveryServer(ConfigDiscoveryConfig{
		Addr:   opdefault.DefaultConfigDiscoveryAddress,
		Logger: zlogger,
	})

	log.Info("starting CDS server")
	ctx, cancel := context.WithCancel(context.Background())
	assert.NoError(t, cds.Start(ctx), "cds server start")
	defer cancel()

	time.Sleep(50 * time.Millisecond)

	logger := logger.NewLoggerFactory(stunnerLogLevel)

	// loader
	id1 := "ns/gw1"
	addr1 := "http://" + opdefault.DefaultConfigDiscoveryAddress
	log.Info("creating CDS client instance for testing Load()", "address", addr1, "id", id1)
	cdsc1, err := cdsclient.NewClient(addr1, id1, logger)
	assert.NoError(t, err, "cds client setup")

	// watcher
	id2 := "ns/gw2"
	addr2 := "ws://" + opdefault.DefaultConfigDiscoveryAddress
	log.Info("creating CDS client instance 1 for testing Watch()", "address", addr2, "id", id2)
	cdsc2, err := cdsclient.NewClient(addr2, id2, logger)
	assert.NoError(t, err, "cds client setup")

	controlCh1 := make(chan stnrconfv1a1.StunnerConfig, 10)
	defer close(controlCh1)
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	err = cdsc2.Watch(ctx2, controlCh1)
	assert.NoError(t, err, "watcher setup")

	time.Sleep(50 * time.Millisecond)

	// we should have a live client connection in the server
	cds.lock.RLock()
	_, ok := cds.conns[id2]
	cds.lock.RUnlock()
	assert.True(t, ok)

	// we should have received an initial zeroconfig
	c0, ok := tryControlCh(controlCh1)
	assert.True(t, ok)
	assert.NotNil(t, c0)
	assert.True(t, cdsclient.ZeroConfig("ns/gw2").DeepEqual(c0), "config ok")

	// loading empty client config errs
	_, err = cdsc1.Load()
	assert.Error(t, err, "loading empty client config errs")

	// we shouldn't have received any more config updates
	_, ok = tryControlCh(controlCh1)
	assert.False(t, ok)

	ch := cds.GetConfigUpdateChannel()

	log.Info("creating a config for the loader", "id", "ns/gw1")
	c1Ok := zeroConfig("ns", "gw1", "realm1")
	e := event.NewEventUpdate(0)
	e.UpsertQueue.ConfigMaps.Reset([]client.Object{packConfig(c1Ok)})
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err := cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	// we shouldn't have received any config updates
	_, ok = tryControlCh(controlCh1)
	assert.False(t, ok)

	// we should have a single config in the store
	assert.Equal(t, 1, cds.store.Len())
	c1m := cds.store.GetObject(store.GetNameFromKey("ns/gw1"))
	c1s, err := store.UnpackConfigMap(c1m)
	assert.NoError(t, err)
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")

	log.Info("creating a config for the 1st watcher", "id", "ns/gw2")
	c2Ok := zeroConfig("ns", "gw2", "realm2")
	e = event.NewEventUpdate(0)
	e.UpsertQueue.ConfigMaps.Reset([]client.Object{packConfig(c1Ok), packConfig(c2Ok)})
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	c2, ok := tryControlCh(controlCh1)
	assert.True(t, ok)
	assert.NotNil(t, c2)
	assert.True(t, c2Ok.DeepEqual(c2), "config ok")

	// we should have 2 configs in the store
	assert.Equal(t, 2, cds.store.Len())
	c1m = cds.store.GetObject(store.GetNameFromKey("ns/gw1"))
	c1s, err = store.UnpackConfigMap(c1m)
	assert.NoError(t, err)
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")
	c2m := cds.store.GetObject(store.GetNameFromKey("ns/gw2"))
	c2s, err := store.UnpackConfigMap(c2m)
	assert.NoError(t, err)
	assert.True(t, c2s.DeepEqual(c2Ok), "config ok")

	log.Info("updating the config of the loader and the 1st watcher with unchanged configs",
		"id1", c1Ok.Admin.Name, "id2", c2Ok.Admin.Name)
	e = event.NewEventUpdate(0)
	e.UpsertQueue.ConfigMaps.Reset([]client.Object{packConfig(c1Ok), packConfig(c2Ok)})
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	// we shouldn't have received an update
	_, ok = tryControlCh(controlCh1)
	assert.False(t, ok)

	log.Info("updating the config of the 1st watcher with a new config", "id", "ns/gw2")
	c2Ok = zeroConfig("ns", "gw2", "realm2_new")
	e = event.NewEventUpdate(0)
	e.UpsertQueue.ConfigMaps.Reset([]client.Object{packConfig(c1Ok), packConfig(c2Ok)})
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	// we should get an update
	c2, ok = tryControlCh(controlCh1)
	assert.True(t, ok)
	assert.NotNil(t, c2)
	assert.True(t, c2Ok.DeepEqual(c2), "config ok")

	// watcher2
	id3 := "ns/gw3"
	log.Info("creating CDS client instance for a 2nd watcher", "address", addr2, "id", id3)
	cdsc3, err := cdsclient.NewClient(addr2, id3, logger)
	assert.NoError(t, err, "cds client setup")

	controlCh2 := make(chan stnrconfv1a1.StunnerConfig, 10)
	defer close(controlCh2)
	ctx3, cancel3 := context.WithCancel(context.Background())
	err = cdsc3.Watch(ctx3, controlCh2)
	assert.NoError(t, err, "watcher setup")

	time.Sleep(50 * time.Millisecond)

	// we should have a live client connection in the server
	cds.lock.RLock()
	_, ok2 := cds.conns[id2]
	_, ok3 := cds.conns[id3]
	cds.lock.RUnlock()
	assert.True(t, ok2)
	assert.True(t, ok3)

	// watcher1 shouldn't have received any config updates
	_, ok = tryControlCh(controlCh1)
	assert.False(t, ok)

	// watcher2 should have received an initial zeroconfig
	c0, ok = tryControlCh(controlCh2)
	assert.True(t, ok)
	assert.NotNil(t, c0)
	assert.True(t, cdsclient.ZeroConfig("ns/gw3").DeepEqual(c0), "config ok")

	log.Info("adding a config CDS for the 2nd watcher", "id", "ns/gw3")
	c3Ok := zeroConfig("ns", "gw3", "realm3_new")
	e = event.NewEventUpdate(0)
	e.UpsertQueue.ConfigMaps.Reset([]client.Object{packConfig(c1Ok), packConfig(c2Ok), packConfig(c3Ok)})
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	// watcher1 shouldn't receive an update
	_, ok = tryControlCh(controlCh1)
	assert.False(t, ok)

	// watcher2 should receive an update
	c3, ok := tryControlCh(controlCh2)
	assert.True(t, ok)
	assert.NotNil(t, c3)
	assert.True(t, c3Ok.DeepEqual(c3), "config ok")

	// we should have 3 configs in the store
	assert.Equal(t, 3, cds.store.Len())
	c1m = cds.store.GetObject(store.GetNameFromKey("ns/gw1"))
	c1s, err = store.UnpackConfigMap(c1m)
	assert.NoError(t, err)
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")
	c2m = cds.store.GetObject(store.GetNameFromKey("ns/gw2"))
	c2s, err = store.UnpackConfigMap(c2m)
	assert.NoError(t, err)
	assert.True(t, c2s.DeepEqual(c2Ok), "config ok")
	c3m := cds.store.GetObject(store.GetNameFromKey("ns/gw3"))
	c3s, err := store.UnpackConfigMap(c3m)
	assert.NoError(t, err)
	assert.True(t, c3s.DeepEqual(c3Ok), "config ok")

	log.Info("removing the config for the 1st watcher", "id", "ns/gw2")
	e = event.NewEventUpdate(0)
	e.UpsertQueue.ConfigMaps.Reset([]client.Object{packConfig(c1Ok), packConfig(c3Ok)})
	ch <- e

	time.Sleep(50 * time.Millisecond)

	c1, err = cdsc1.Load()
	assert.NoError(t, err, "loading client config ok")
	assert.True(t, c1Ok.DeepEqual(c1), "config ok")

	// watcher1 should receive a zeroconfig
	c2, ok = tryControlCh(controlCh1)
	assert.True(t, ok)
	assert.NotNil(t, c2)
	assert.True(t, c2.DeepEqual(cdsclient.ZeroConfig("ns/gw2")), "config ok")

	// watcher2 should not receive anything
	_, ok = tryControlCh(controlCh2)
	assert.False(t, ok)

	// we should have 2 configs in the store
	assert.Equal(t, 2, cds.store.Len())
	c1m = cds.store.GetObject(store.GetNameFromKey("ns/gw1"))
	c1s, err = store.UnpackConfigMap(c1m)
	assert.NoError(t, err)
	assert.True(t, c1s.DeepEqual(c1Ok), "config ok")
	c3m = cds.store.GetObject(store.GetNameFromKey("ns/gw3"))
	c3s, err = store.UnpackConfigMap(c3m)
	assert.NoError(t, err)
	assert.True(t, c3s.DeepEqual(c3Ok), "config ok")

	log.Info("closing the 2nd watcher", "id", "nw/gw3")
	cancel3()

	time.Sleep(50 * time.Millisecond)

	// should remove the 2nd watcher's client connection in the server
	// NOTE: this will not pass since the server does not check wether the client is alive
	// cds.lock.RLock()
	// _, ok = cds.conns[id3]
	// cds.lock.RUnlock()
	// assert.False(t, ok)

	log.Info("reinstalling the 2nd watcher", "id", "nw/gw3")
	// should yield a zero config plus a real config
	controlCh2 = make(chan stnrconfv1a1.StunnerConfig, 10)
	defer close(controlCh2)
	ctx3, cancel3 = context.WithCancel(context.Background())
	defer cancel3()
	err = cdsc3.Watch(ctx3, controlCh2)
	assert.NoError(t, err, "watcher setup")

	time.Sleep(50 * time.Millisecond)

	// we should have a live client connection in the server
	cds.lock.RLock()
	_, ok = cds.conns[id3]
	cds.lock.RUnlock()
	assert.True(t, ok)

	// we should have received a valid config
	c3, ok = tryControlCh(controlCh2)
	assert.True(t, ok)
	assert.NotNil(t, c3)
	assert.True(t, c3.DeepEqual(c3Ok), "config ok")

	log.Info("closing the connection of the 2nd watcher", "id", "nw/gw3")
	cds.lock.RLock()
	conn, ok := cds.conns[id3]
	cds.lock.RUnlock()
	assert.True(t, ok)
	cds.lock.Lock()
	delete(cds.conns, id3)
	cds.lock.Unlock()
	conn.Close()

	// after 2 pong-waits, client should have reconnected
	time.Sleep(cdsclient.RetryPeriod)
	time.Sleep(cdsclient.RetryPeriod)

	// we should have a connection to the 2nd watcher
	cds.lock.RLock()
	_, ok = cds.conns[id2]
	cds.lock.RUnlock()
	assert.True(t, ok)

	// 2nd watcher should receive its config
	c3, ok = tryControlCh(controlCh2)
	assert.True(t, ok)
	assert.NotNil(t, c3)
	assert.True(t, c3.DeepEqual(c3Ok), "config ok")

	log.Info("removing all configs")
	e = event.NewEventUpdate(0)
	e.UpsertQueue.ConfigMaps.Reset([]client.Object{})
	ch <- e

	time.Sleep(50 * time.Millisecond)

	_, err = cdsc1.Load()
	assert.Error(t, err, "loading client config errs")

	// watcher2 should receive a zeroconfig
	c3, ok = tryControlCh(controlCh2)
	assert.True(t, ok)
	assert.NotNil(t, c3)
	assert.True(t, c3.DeepEqual(cdsclient.ZeroConfig("ns/gw3")), "config ok")

	// we should have no configs in the store
	assert.Equal(t, 0, cds.store.Len())
}

func zeroConfig(namespace, name, realm string) *stnrconfv1a1.StunnerConfig {
	id := fmt.Sprintf("%s/%s", namespace, name)
	c := cdsclient.ZeroConfig(id)
	c.Auth.Realm = realm

	return c
}

func packConfig(c *stnrconfv1a1.StunnerConfig) *corev1.ConfigMap {
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

func tryControlCh(controlCh chan stnrconfv1a1.StunnerConfig) (*stnrconfv1a1.StunnerConfig, bool) {
	select {
	case c, ok := <-controlCh:
		return &c, ok
	default:
		return nil, false
	}
}
