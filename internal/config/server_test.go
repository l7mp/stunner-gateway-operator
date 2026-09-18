package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	stnrapiv1 "github.com/l7mp/stunner/v2/pkg/apis/v1"
	cdsclient "github.com/l7mp/stunner/v2/pkg/config/client"
	cdsserver "github.com/l7mp/stunner/v2/pkg/config/server"
	"github.com/l7mp/stunner/v2/pkg/logger"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// var testerLogLevel = zapcore.Level(-4)
// var testerLogLevel = zapcore.DebugLevel
var testerLogLevel = zapcore.ErrorLevel

// const stunnerTestLoglevel = "all:TRACE"
const stunnerTestLoglevel = "all:ERROR"

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

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "config")
}

var _ = Describe("Config discovery", Ordered, func() {
	var (
		log                 logr.Logger
		srv                 *Server
		ch                  chan event.Event
		ctx, ctx2           context.Context
		cancel2             context.CancelFunc
		loggerFactory       logger.LoggerFactory
		addr1, addr2        string
		cdsc1, cdsc2, cdsc3 cdsclient.Client
		ch1, ch2, ch3       chan *stnrapiv1.StunnerConfig
		c1Ok, c2Ok, c3Ok    *stnrapiv1.StunnerConfig
		lc                  cdsclient.LicenseStatusClient
		licenseStatus       stnrapiv1.LicenseStatus
		connIds             []string
	)

	BeforeAll(func() {
		zc := zap.NewProductionConfig()
		zc.Level = zap.NewAtomicLevelAt(testerLogLevel)
		z, err := zc.Build()
		Expect(err).To(Succeed(), "loggerFactory created")
		zlogger := zapr.NewLogger(z)
		log = zlogger.WithName("tester")

		// setup a fast pinger so that we get a timely error notification
		cdsclient.PingPeriod = 200 * time.Millisecond
		cdsclient.PongWait = 300 * time.Millisecond
		cdsclient.WriteWait = 400 * time.Millisecond
		cdsclient.RetryPeriod = 400 * time.Millisecond

		nodeStore := store.NewNodeStore()
		n1 := testutils.TestNode.DeepCopy()
		nodeStore.Upsert(n1)

		testCDSAddr := getRandCDSAddr()
		log.Info("create server", "address", testCDSAddr)
		patcher := func(conf *stnrapiv1.StunnerConfig, node string) *stnrapiv1.StunnerConfig {
			if n := nodeStore.GetObject(types.NamespacedName{Name: node}); n != nil {
				// rewrite the realm to the node name
				for _, a := range n.Status.Addresses {
					if a.Type == corev1.NodeExternalIP {
						conf.Auth.Realm = a.Address
						return conf
					}
				}
			}
			return conf
		}
		cdslog := zlogger.WithName("cds-server")
		srv = &Server{
			Server:          cdsserver.New(testCDSAddr, patcher, cdslog),
			configCh:        make(chan event.Event, 10),
			ProgressTracker: NewProgressTracker(),
			log:             cdslog,
		}

		log.Info("starting CDS server")
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
		go func() { Expect(srv.Start(ctx)).To(Succeed(), "cds server start") }()
		ch = srv.GetConfigUpdateChannel()

		time.Sleep(50 * time.Millisecond)
		loggerFactory = logger.NewLoggerFactory(stunnerTestLoglevel)

		id1 := "ns/gw1"
		addr1 = "http://" + testCDSAddr
		log.Info("creating CDS client instance 1", "address", addr1, "id", id1)
		cdsc1, err = cdsclient.New(addr1, id1, "testnode-ok", loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		id2 := "ns/gw2"
		addr2 = "http://" + testCDSAddr
		log.Info("creating CDS client instance 2", "address", addr2, "id", id2)
		cdsc2, err = cdsclient.New(addr2, id2, "", loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		ch1 = make(chan *stnrapiv1.StunnerConfig, 10)
		ch2 = make(chan *stnrapiv1.StunnerConfig, 10)
		err = cdsc1.Watch(ctx, ch1, true)
		Expect(err).To(Succeed(), "watcher setup 1")
		err = cdsc2.Watch(ctx, ch2, true)
		Expect(err).To(Succeed(), "watcher setup 2")

		time.Sleep(50 * time.Millisecond)

	})

	It("should track both client connections and serve no config yet", func() {
		// we should now have 2 client connections
		conns := srv.GetConnTrack()
		Expect(conns).NotTo(BeNil())
		snapshot := conns.Snapshot()
		Expect(snapshot).To(HaveLen(2))

		// loading empty client config errs
		_, err := cdsc1.Load()
		Expect(err).To(HaveOccurred(), "loading empty client config errs")
		_, err = cdsc2.Load()
		Expect(err).To(HaveOccurred(), "loading empty client config errs")

		// we shouldn't have received any config updates
		c1 := watchConfig(ch1, 500*time.Millisecond)
		Expect(c1).To(BeNil())
		c2 := watchConfig(ch1, 500*time.Millisecond)
		Expect(c2).To(BeNil())

	})

	It("should serve the first config to its own client only", func() {
		log.Info("creating a config for the loader", "id", "ns/gw1")
		c1Ok = zeroConfig("ns", "gw1", "realm1")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv1.StunnerConfig{c1Ok}
		ch <- e

		time.Sleep(50 * time.Millisecond)

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Auth.Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auth.Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")

		c2, err := cdsc2.Load()
		Expect(err).To(HaveOccurred(), "load")
		Expect(c2).To(BeNil())

		// we should have received a config update
		c1 = watchConfig(ch1, 1500*time.Millisecond)
		Expect(c1).NotTo(BeNil())
		Expect(c1.Auth.Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auth.Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")

		// no config update from client 2
		c2 = watchConfig(ch2, 150*time.Millisecond)
		Expect(c2).To(BeNil())

		// we should have a single config in the store
		cStore := srv.GetConfigStore()
		Expect(len(cStore.Snapshot())).To(Equal(1))
		c1s, ok := cStore.Get("ns", "gw1")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c1s.Config.DeepEqual(c1Ok)).To(BeTrue(), "config ok")

		// license status client should return a nil license status
		lc, err = cdsclient.NewLicenseStatusClient(addr1, loggerFactory.NewLogger("license-status"))
		Expect(err).To(Succeed(), "license client setup")
		status, err := lc.LicenseStatus(ctx)
		Expect(err).To(Succeed(), "loading status 1 ok")
		Expect(status).To(Equal(stnrapiv1.NewEmptyLicenseStatus()), "license 1 ok")

	})

	It("should serve a second config, and the license status, to the second client", func() {
		log.Info("creating a config for the 2nd client", "id", "ns/gw2")
		c2Ok = zeroConfig("ns", "gw2", "realm2")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv1.StunnerConfig{c1Ok, c2Ok}
		licenseStatus = stnrapiv1.LicenseStatus{
			EnabledFeatures:  []string{"a", "b", "c"},
			SubscriptionType: "test",
			LastUpdated:      "never",
			LastError:        "",
		}
		e.LicenseStatus = licenseStatus
		ch <- e

		time.Sleep(50 * time.Millisecond)

		// license status client should return the new license status
		status, err := lc.LicenseStatus(ctx)
		Expect(err).To(Succeed(), "loading status 1 ok")
		Expect(status).To(Equal(licenseStatus), "license 1 ok")

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client 1 config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Auth.Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auth.Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err := cdsc2.Load()
		Expect(err).To(Succeed(), "loading client 2 config ok")
		Expect(c2).NotTo(BeNil())
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

		c1 = watchConfig(ch1, 150*time.Millisecond)
		Expect(c1).To(BeNil())
		c2 = watchConfig(ch2, 1500*time.Millisecond)
		Expect(c2).NotTo(BeNil())
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

		// we should have 2 configs in the store
		cStore := srv.GetConfigStore()
		Expect(len(cStore.Snapshot())).To(Equal(2))
		c1s, ok := cStore.Get("ns", "gw1")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c1s).NotTo(BeNil())
		Expect(c1s.Config.DeepEqual(c1Ok)).To(BeTrue(), "config ok")
		c2s, ok := cStore.Get("ns", "gw2")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c2s).NotTo(BeNil())
		Expect(c2s.Config.DeepEqual(c2Ok)).To(BeTrue(), "config ok")

	})

	It("should push an updated config to the affected client only", func() {
		log.Info("updating the 2nd config", "id2", c2Ok.Admin.Name)
		c2Ok = zeroConfig("ns", "gw2", "realm2-new")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv1.StunnerConfig{c1Ok, c2Ok}
		ch <- e

		time.Sleep(50 * time.Millisecond)

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Auth.Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auth.Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err := cdsc2.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c2).NotTo(BeNil())
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

		c1 = watchConfig(ch1, 150*time.Millisecond)
		Expect(c1).To(BeNil())
		c2 = watchConfig(ch2, 1500*time.Millisecond)
		Expect(c2).NotTo(BeNil())
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

	})

	It("should accept a third watcher without disturbing the others", func() {
		// watcher3
		id3 := "ns/gw3"
		log.Info("creating CDS client instance 3", "address", addr2, "id", id3)
		var err error
		cdsc3, err = cdsclient.New(addr2, id3, "", loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		ch3 = make(chan *stnrapiv1.StunnerConfig, 10)
		ctx2, cancel2 = context.WithCancel(context.Background())
		err = cdsc3.Watch(ctx2, ch3, false)
		Expect(err).To(Succeed(), "watcher setup")

		time.Sleep(50 * time.Millisecond)

		// we should now have 3 client connections: store IDs for later use
		conns := srv.GetConnTrack()
		Expect(conns).NotTo(BeNil())
		snapshot := conns.Snapshot()
		Expect(snapshot).To(HaveLen(3))
		connIds = []string{}
		for _, conn := range snapshot {
			connIds = append(connIds, conn.Id())
		}

		c1 := watchConfig(ch1, 1500*time.Millisecond)
		Expect(c1).To(BeNil())
		c2 := watchConfig(ch2, 150*time.Millisecond)
		Expect(c2).To(BeNil())
		c3 := watchConfig(ch3, 150*time.Millisecond)
		Expect(c3).To(BeNil())

	})

	It("should serve the third config to the third client only", func() {
		log.Info("adding a config CDS for the 3rd client", "id", "ns/gw3")
		c3Ok = zeroConfig("ns", "gw3", "realm3_new")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv1.StunnerConfig{c1Ok, c2Ok, c3Ok}
		ch <- e

		time.Sleep(50 * time.Millisecond)

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Auth.Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auth.Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err := cdsc2.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c2).NotTo(BeNil())
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")
		c3, err := cdsc3.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c3).NotTo(BeNil())
		Expect(c3Ok.DeepEqual(c3)).To(BeTrue(), "config ok")

		// watcher1 shouldn't receive an update
		c1 = watchConfig(ch1, 1500*time.Millisecond)
		Expect(c1).To(BeNil())
		c1 = watchConfig(ch1, 1500*time.Millisecond)
		Expect(c1).To(BeNil())
		c3 = watchConfig(ch3, 150*time.Millisecond)
		Expect(c3).NotTo(BeNil())
		Expect(c3Ok.DeepEqual(c3)).To(BeTrue(), "config ok")

		// we should have 3 configs in the store
		cStore := srv.GetConfigStore()
		Expect(len(cStore.Snapshot())).To(Equal(3))
		c1s, ok := cStore.Get("ns", "gw1")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c1s).NotTo(BeNil())
		Expect(c1s.Config.DeepEqual(c1Ok)).To(BeTrue(), "config ok")
		c2s, ok := cStore.Get("ns", "gw2")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c2s).NotTo(BeNil())
		Expect(c2s.Config.DeepEqual(c2Ok)).To(BeTrue(), "config ok")
		c3s, ok := cStore.Get("ns", "gw3")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c3s).NotTo(BeNil())
		Expect(c3s.Config.DeepEqual(c3Ok)).To(BeTrue(), "config ok")

	})

	It("should stop serving a config that was removed", func() {
		log.Info("removing the config for the 2nd client", "id", "ns/gw2")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv1.StunnerConfig{c1Ok, c3Ok}
		ch <- e

		time.Sleep(50 * time.Millisecond)

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Auth.Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auth.Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err := cdsc2.Load()
		Expect(err).To(HaveOccurred(), "load")
		Expect(c2).To(BeNil())
		c3, err := cdsc3.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c3).NotTo(BeNil())
		Expect(c3Ok.DeepEqual(c3)).To(BeTrue(), "config ok")

		// watcher2 should have received nothing (deleted configs are not updated)
		c1 = watchConfig(ch1, 150*time.Millisecond)
		Expect(c1).To(BeNil())
		c2 = watchConfig(ch2, 150*time.Millisecond)
		Expect(c2).To(BeNil())
		c3 = watchConfig(ch3, 150*time.Millisecond)
		Expect(c3).To(BeNil())

		// we should have 2 configs in the store
		cStore := srv.GetConfigStore()
		Expect(len(cStore.Snapshot())).To(Equal(2))
		c1s, ok := cStore.Get("ns", "gw1")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c1s).NotTo(BeNil())
		Expect(c1s.Config.DeepEqual(c1Ok)).To(BeTrue(), "config ok")
		c3s, ok := cStore.Get("ns", "gw3")
		Expect(ok).To(BeTrue(), "config ok")
		Expect(c3s).NotTo(BeNil())
		Expect(c3s.Config.DeepEqual(c3Ok)).To(BeTrue(), "config ok")

	})

	AfterAll(func() {
		if cancel2 != nil {
			cancel2()
		}
	})

	It("should re-serve the config when a watcher reconnects", func() {
		log.Info("closing the 3rd watcher", "id", "nw/gw3")
		cancel2()
		time.Sleep(50 * time.Millisecond)

		log.Info("reinstalling the 2nd watcher", "id", "nw/gw3")
		ch3 = make(chan *stnrapiv1.StunnerConfig, 10)
		ctx2, cancel2 = context.WithCancel(context.Background())
		err := cdsc3.Watch(ctx2, ch3, false)
		Expect(err).To(Succeed(), "watcher setup")
		time.Sleep(50 * time.Millisecond)

		// we should have received a valid config
		c3 := watchConfig(ch3, 1500*time.Millisecond)
		Expect(c3).NotTo(BeNil())
		Expect(c3.DeepEqual(c3Ok)).To(BeTrue(), "config ok")

	})

	It("should re-serve the config when the server drops the connection", func() {
		log.Info("closing the connection of the 3rd watcher", "id", "nw/gw3")
		conns := srv.GetConnTrack()
		Expect(conns).NotTo(BeNil())
		snapshot := conns.Snapshot()
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
		c3 := watchConfig(ch3, 150*time.Millisecond)
		Expect(c3).NotTo(BeNil())
		Expect(c3.DeepEqual(c3Ok)).To(BeTrue(), "config ok")

	})

	It("should empty the store when every config is removed", func() {
		log.Info("removing all configs")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv1.StunnerConfig{}
		ch <- e

		time.Sleep(50 * time.Millisecond)

		_, err := cdsc1.Load()
		Expect(err).To(HaveOccurred(), "loading client config errs")

		// watcher2 should have received nothing
		c2 := watchConfig(ch2, 150*time.Millisecond)
		Expect(c2).To(BeNil())

		// we should have no configs in the store
		cStore := srv.GetConfigStore()
		Expect(len(cStore.Snapshot())).To(Equal(0))
	})
})

var _ = Describe("Config patcher", Ordered, func() {
	var (
		log           logr.Logger
		srv           *Server
		loggerFactory logger.LoggerFactory
		addr1, id1    string
		config        *stnrapiv1.StunnerConfig
	)

	BeforeAll(func() {
		zc := zap.NewProductionConfig()
		zc.Level = zap.NewAtomicLevelAt(testerLogLevel)
		z, err := zc.Build()
		Expect(err).To(Succeed(), "loggerFactory created")
		zlogger := zapr.NewLogger(z)
		log = zlogger.WithName("tester")

		n1 := testutils.TestNode.DeepCopy()
		n1.SetName("testnode1")
		store.Nodes.Upsert(n1)
		n2 := testutils.TestNode.DeepCopy()
		n2.SetName("testnode2")
		n2.Status.Addresses = []corev1.NodeAddress{{
			Type:    corev1.NodeInternalIP,
			Address: "1.2.3.5",
		}, {
			Type:    corev1.NodeExternalDNS,
			Address: "google.com",
		}}
		store.Nodes.Upsert(n2)

		config = &stnrapiv1.StunnerConfig{
			ApiVersion: stnrapiv1.ApiVersion,
			Admin: stnrapiv1.AdminConfig{
				Name:     "ns/gw1",
				LogLevel: stunnerTestLoglevel,
			},
			Auth: stnrapiv1.AuthConfig{
				Credentials: map[string]string{
					"username": "user",
					"password": "pass",
				},
			},
			Listeners: []stnrapiv1.ListenerConfig{{
				Name: "default-listener",
				Addr: opdefault.DefaultSTUNnerAddressEnvVarName,
			}},
		}

		testCDSAddr := getRandCDSAddr()
		log.Info("create server", "address", testCDSAddr)
		srv = NewCDSServer(testCDSAddr, zlogger.WithName("cds-server"))
		addr1, id1 = "http://"+testCDSAddr, "ns/gw1"
		Expect(srv).NotTo(BeNil(), "CDS server")

		log.Info("starting CDS server")
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		go func() { Expect(srv.Start(ctx)).To(Succeed(), "cds server start") }()
		time.Sleep(50 * time.Millisecond)

		loggerFactory = logger.NewLoggerFactory(stunnerTestLoglevel)

	})

	AfterAll(func() { store.Nodes.Flush() })

	It("should patch the placeholder with the node's external IP", func() {
		// first client uses testnode1
		log.Info("creating CDS client instance 1", "address", addr1, "id", id1)
		cdsc1, err := cdsclient.New(addr1, id1, "testnode1", loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		log.Info("load default config -> no patch")
		config.Listeners[0].Addr = opdefault.DefaultSTUNnerAddressEnvVarName
		Expect(srv.UpdateConfig([]cdsserver.Config{{
			Name:      "gw1",
			Namespace: "ns",
			Config:    config,
		}})).To(Succeed(), "config server update")

		Eventually(func() bool {
			c, err := cdsc1.Load()
			if err != nil {
				// transient load failure: let Eventually retry instead of tripping on a
				// nil config
				return false
			}
			return len(c.Listeners) == 1 && c.Listeners[0].Addr == opdefault.DefaultSTUNnerAddressEnvVarName
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

		log.Info("load config that requires node name patching -> patched with testnode1 external IP")
		config.Listeners[0].Addr = opdefault.NodeAddressPlaceholder
		Expect(srv.UpdateConfig([]cdsserver.Config{{
			Name:      "gw1",
			Namespace: "ns",
			Config:    config,
		}})).To(Succeed(), "config server update")

		Eventually(func() bool {
			c, err := cdsc1.Load()
			if err != nil {
				// transient load failure: let Eventually retry instead of tripping on a
				// nil config
				return false
			}
			return len(c.Listeners) == 1 && c.Listeners[0].Addr == "1.2.3.4" // testnode 1 external ip
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

	})

	It("should patch the placeholder with the node's external DNS name", func() {
		// second client uses testnode2 -> external DNS!
		log.Info("creating CDS client instance 2", "address", addr1, "id", id1)
		cdsc1, err := cdsclient.New(addr1, id1, "testnode2", loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		log.Info("load default config -> no patch")
		config.Listeners[0].Addr = opdefault.DefaultSTUNnerAddressEnvVarName
		Expect(srv.UpdateConfig([]cdsserver.Config{{
			Name:      "gw1",
			Namespace: "ns",
			Config:    config,
		}})).To(Succeed(), "config server update")

		Eventually(func() bool {
			c, err := cdsc1.Load()
			if err != nil {
				// transient load failure: let Eventually retry instead of tripping on a
				// nil config
				return false
			}
			return len(c.Listeners) == 1 && c.Listeners[0].Addr == opdefault.DefaultSTUNnerAddressEnvVarName
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

		log.Info("load config that requires node name patching -> patched with testnode2 external DNS")
		config.Listeners[0].Addr = opdefault.NodeAddressPlaceholder
		Expect(srv.UpdateConfig([]cdsserver.Config{{
			Name:      "gw1",
			Namespace: "ns",
			Config:    config,
		}})).To(Succeed(), "config server update")

		Eventually(func() bool {
			c, err := cdsc1.Load()
			if err != nil {
				// transient load failure: let Eventually retry instead of tripping on a
				// nil config
				return false
			}
			return len(c.Listeners) == 1 && net.ParseIP(c.Listeners[0].Addr) != nil // testnode2 addr should parse as ip
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

	})

	It("should leave the placeholder alone for an unknown node", func() {
		// third client uses unknown node -> no patching!
		log.Info("creating CDS client instance 3", "address", addr1, "id", id1)
		cdsc1, err := cdsclient.New(addr1, id1, "dummy-node", loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		log.Info("load default config -> no patch")
		config.Listeners[0].Addr = opdefault.DefaultSTUNnerAddressEnvVarName
		Expect(srv.UpdateConfig([]cdsserver.Config{{
			Name:      "gw1",
			Namespace: "ns",
			Config:    config,
		}})).To(Succeed(), "config server update")

		Eventually(func() bool {
			c, err := cdsc1.Load()
			if err != nil {
				// transient load failure: let Eventually retry instead of tripping on a
				// nil config
				return false
			}
			return len(c.Listeners) == 1 && c.Listeners[0].Addr == opdefault.DefaultSTUNnerAddressEnvVarName
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

		log.Info("load config that requires node name patching -> should not be patched as node does not exist")
		config.Listeners[0].Addr = opdefault.NodeAddressPlaceholder
		Expect(srv.UpdateConfig([]cdsserver.Config{{
			Name:      "gw1",
			Namespace: "ns",
			Config:    config,
		}})).To(Succeed(), "config server update")

		Eventually(func() bool {
			c, err := cdsc1.Load()
			if err != nil {
				// transient load failure: let Eventually retry instead of tripping on a
				// nil config
				return false
			}
			return len(c.Listeners) == 1 && c.Listeners[0].Addr == opdefault.DefaultSTUNnerAddressEnvVarName
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

	})
})

var _ = Describe("getNodeAddress", func() {
	Context("When the node advertises an IPv6 ExternalIP", func() {
		It("should return it unbracketed", func() {
			n := testutils.TestNode.DeepCopy()
			n.SetName("ipv6node")
			n.Status.Addresses = []corev1.NodeAddress{{
				Type:    corev1.NodeExternalIP,
				Address: "2001:db8::1",
			}}
			store.Nodes.Upsert(n)
			DeferCleanup(store.Nodes.Flush)

			aType, addr, err := getNodeAddress("ipv6node")
			Expect(err).To(Succeed(), "resolve IPv6 node address")
			Expect(aType).To(Equal(corev1.NodeExternalIP), "address type")
			Expect(addr).To(Equal("2001:db8::1"), "IPv6 external address")
		})
	})
})

// wait for some configurable time for a watch element
func watchConfig(ch chan *stnrapiv1.StunnerConfig, d time.Duration) *stnrapiv1.StunnerConfig {
	select {
	case c := <-ch:
		// fmt.Println("++++++++++++ got config ++++++++++++: ", c.String())
		return c
	case <-time.After(d):
		// fmt.Println("++++++++++++ timeout ++++++++++++")
		return nil
	}
}

// getRandCDSAddr returns a loopback address with a kernel-allocated free port for a test CDS
// server: a blind random port may collide with the ephemeral sockets of a previous test phase.
func getRandCDSAddr() string {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer probe.Close()
	return fmt.Sprintf("127.0.0.1:%d", probe.Addr().(*net.TCPAddr).Port)
}

func zeroConfig(namespace, name, realm string) *stnrapiv1.StunnerConfig {
	id := fmt.Sprintf("%s/%s", namespace, name)
	c := cdsclient.ZeroConfig(id)
	c.Auth.Realm = realm
	_ = c.Validate()
	return c
}

//nolint:unused
func packConfig(c *stnrapiv1.StunnerConfig) *corev1.ConfigMap {
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
