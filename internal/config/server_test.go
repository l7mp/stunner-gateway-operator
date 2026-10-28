package config

import (
	"context"
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
	"k8s.io/apimachinery/pkg/types"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
	cdsclient "github.com/l7mp/stunner/v2/pkg/config/client"
	cdsserver "github.com/l7mp/stunner/v2/pkg/config/server"
	"github.com/l7mp/stunner/v2/pkg/logger"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/event"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"
)

// var testerLogLevel = zapcore.Level(-4)
// var testerLogLevel = zapcore.DebugLevel
var testerLogLevel = zapcore.ErrorLevel

// const stunnerTestLoglevel = "all:TRACE"
const stunnerTestLoglevel = "all:ERROR"

// settle bounds every wait for something that must happen: a starved CI runner may take seconds,
// a fast machine returns at once.
const settle = 5 * time.Second

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
		ch1, ch2, ch3       chan *stnrapiv2.StunnerConfig
		c1Ok, c2Ok, c3Ok    *stnrapiv2.StunnerConfig
		lc                  cdsclient.LicenseStatusClient
		licenseStatus       stnrapiv2.LicenseStatus
		connIds             []string
	)

	BeforeAll(func() {
		zc := zap.NewProductionConfig()
		zc.Level = zap.NewAtomicLevelAt(testerLogLevel)
		z, err := zc.Build()
		Expect(err).To(Succeed(), "loggerFactory created")
		zlogger := zapr.NewLogger(z)
		log = zlogger.WithName("tester")

		// reconnect fast after a dropped connection; the pinger keeps its defaults, since a tight
		// pong deadline drops healthy connections on a slow machine and re-serves their configs
		cdsclient.RetryPeriod = 400 * time.Millisecond

		nodeStore := store.NewNodeStore()
		n1 := testutils.TestNode.DeepCopy()
		nodeStore.Upsert(n1)

		testCDSAddr := getRandCDSAddr()
		log.Info("create server", "address", testCDSAddr)
		patcher := func(conf *stnrapiv2.StunnerConfig, _ string, labels map[string]string) *stnrapiv2.StunnerConfig {
			node := labels[stnrapiv2.DefaultCDSNodeLabel]
			if n := nodeStore.GetObject(types.NamespacedName{Name: node}); n != nil {
				// rewrite the realm to the node's external IP
				for _, a := range n.Status.Addresses {
					if a.Type == corev1.NodeExternalIP {
						conf.Auths[0].Realm = a.Address
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

		loggerFactory = logger.NewLoggerFactory(stunnerTestLoglevel)

		id1 := "ns/gw1"
		addr1 = "http://" + testCDSAddr
		log.Info("creating CDS client instance 1", "address", addr1, "id", id1)
		cdsc1, err = cdsclient.New(addr1, id1, map[string]string{stnrapiv2.DefaultCDSNodeLabel: "testnode-ok"}, loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		id2 := "ns/gw2"
		addr2 = "http://" + testCDSAddr
		log.Info("creating CDS client instance 2", "address", addr2, "id", id2)
		cdsc2, err = cdsclient.New(addr2, id2, nil, loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		ch1 = make(chan *stnrapiv2.StunnerConfig, 10)
		ch2 = make(chan *stnrapiv2.StunnerConfig, 10)
		err = cdsc1.Watch(ctx, ch1, true)
		Expect(err).To(Succeed(), "watcher setup 1")
		err = cdsc2.Watch(ctx, ch2, true)
		Expect(err).To(Succeed(), "watcher setup 2")
	})

	// connCount is the number of client connections the server tracks.
	connCount := func() int { return len(srv.GetConnTrack().Snapshot()) }

	It("should track both client connections and serve no config yet", func() {
		// we should now have 2 client connections
		Eventually(connCount).WithTimeout(settle).Should(Equal(2))

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
		e.ConfigQueue = []*stnrapiv2.StunnerConfig{c1Ok}
		ch <- e

		// the watcher receiving the update shows that the server took the config
		c1 := watchConfig(ch1, settle)
		Expect(c1).NotTo(BeNil())
		Expect(c1.Auths[0].Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auths[0].Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Validate()).To(Succeed(), "the client does not validate")
		Expect(c1.Auths[0].Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auths[0].Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")

		c2, err := cdsc2.Load()
		Expect(err).To(HaveOccurred(), "load")
		Expect(c2).To(BeNil())

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
		Expect(status).To(Equal(stnrapiv2.NewEmptyLicenseStatus()), "license 1 ok")

	})

	It("should serve a second config, and the license status, to the second client", func() {
		log.Info("creating a config for the 2nd client", "id", "ns/gw2")
		c2Ok = zeroConfig("ns", "gw2", "realm2")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv2.StunnerConfig{c1Ok, c2Ok}
		licenseStatus = stnrapiv2.LicenseStatus{
			EnabledFeatures:  []string{"a", "b", "c"},
			SubscriptionType: "test",
			LastUpdated:      "never",
			LastError:        "",
		}
		e.LicenseStatus = licenseStatus
		ch <- e

		// the watcher receiving the update shows that the server took the config
		c2 := watchConfig(ch2, settle)
		Expect(c2).NotTo(BeNil())
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

		// license status client should return the new license status
		status, err := lc.LicenseStatus(ctx)
		Expect(err).To(Succeed(), "loading status 1 ok")
		Expect(status).To(Equal(licenseStatus), "license 1 ok")

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client 1 config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Validate()).To(Succeed(), "the client does not validate")
		Expect(c1.Auths[0].Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auths[0].Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err = cdsc2.Load()
		Expect(err).To(Succeed(), "loading client 2 config ok")
		Expect(c2).NotTo(BeNil())
		Expect(c2.Validate()).To(Succeed(), "the client does not validate")
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

		c1 = watchConfig(ch1, 150*time.Millisecond)
		Expect(c1).To(BeNil())

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
		e.ConfigQueue = []*stnrapiv2.StunnerConfig{c1Ok, c2Ok}
		ch <- e

		// the watcher receiving the update shows that the server took the config
		c2 := watchConfig(ch2, settle)
		Expect(c2).NotTo(BeNil())
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Validate()).To(Succeed(), "the client does not validate")
		Expect(c1.Auths[0].Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auths[0].Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err = cdsc2.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c2).NotTo(BeNil())
		Expect(c2.Validate()).To(Succeed(), "the client does not validate")
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")

		c1 = watchConfig(ch1, 150*time.Millisecond)
		Expect(c1).To(BeNil())
	})

	It("should accept a third watcher without disturbing the others", func() {
		// watcher3
		id3 := "ns/gw3"
		log.Info("creating CDS client instance 3", "address", addr2, "id", id3)
		var err error
		cdsc3, err = cdsclient.New(addr2, id3, nil, loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		ch3 = make(chan *stnrapiv2.StunnerConfig, 10)
		ctx2, cancel2 = context.WithCancel(context.Background())
		err = cdsc3.Watch(ctx2, ch3, false)
		Expect(err).To(Succeed(), "watcher setup")

		// we should now have 3 client connections: store IDs for later use
		Eventually(connCount).WithTimeout(settle).Should(Equal(3))
		connIds = []string{}
		for _, conn := range srv.GetConnTrack().Snapshot() {
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
		e.ConfigQueue = []*stnrapiv2.StunnerConfig{c1Ok, c2Ok, c3Ok}
		ch <- e

		// the watcher receiving the update shows that the server took the config
		c3 := watchConfig(ch3, settle)
		Expect(c3).NotTo(BeNil())
		Expect(c3Ok.DeepEqual(c3)).To(BeTrue(), "config ok")

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Validate()).To(Succeed(), "the client does not validate")
		Expect(c1.Auths[0].Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auths[0].Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err := cdsc2.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c2).NotTo(BeNil())
		Expect(c2.Validate()).To(Succeed(), "the client does not validate")
		Expect(c2Ok.DeepEqual(c2)).To(BeTrue(), "config ok")
		c3, err = cdsc3.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c3).NotTo(BeNil())
		Expect(c3.Validate()).To(Succeed(), "the client does not validate")
		Expect(c3Ok.DeepEqual(c3)).To(BeTrue(), "config ok")

		// watcher1 shouldn't receive an update
		c1 = watchConfig(ch1, 1500*time.Millisecond)
		Expect(c1).To(BeNil())

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
		e.ConfigQueue = []*stnrapiv2.StunnerConfig{c1Ok, c3Ok}
		ch <- e

		// a removed config is pushed to nobody: wait for the server to stop serving it
		Eventually(func() error { _, err := cdsc2.Load(); return err }).WithTimeout(settle).Should(HaveOccurred())

		c1, err := cdsc1.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c1).NotTo(BeNil())
		Expect(c1.Validate()).To(Succeed(), "the client does not validate")
		Expect(c1.Auths[0].Realm).To(Equal("1.2.3.4"), "node name ok")
		c1.Auths[0].Realm = "realm1" // reset
		Expect(c1Ok.DeepEqual(c1)).To(BeTrue(), "config ok")
		c2, err := cdsc2.Load()
		Expect(err).To(HaveOccurred(), "load")
		Expect(c2).To(BeNil())
		c3, err := cdsc3.Load()
		Expect(err).To(Succeed(), "loading client config ok")
		Expect(c3).NotTo(BeNil())
		Expect(c3.Validate()).To(Succeed(), "the client does not validate")
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
		Eventually(connCount).WithTimeout(settle).Should(Equal(2))

		log.Info("reinstalling the 2nd watcher", "id", "nw/gw3")
		ch3 = make(chan *stnrapiv2.StunnerConfig, 10)
		ctx2, cancel2 = context.WithCancel(context.Background())
		err := cdsc3.Watch(ctx2, ch3, false)
		Expect(err).To(Succeed(), "watcher setup")

		// we should have received a valid config
		c3 := watchConfig(ch3, settle)
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

		// the 3rd watcher reconnects after a retry period and receives its config
		c3 := watchConfig(ch3, settle)
		Expect(c3).NotTo(BeNil())
		Expect(c3.DeepEqual(c3Ok)).To(BeTrue(), "config ok")

	})

	It("should empty the store when every config is removed", func() {
		log.Info("removing all configs")
		e := event.NewEventUpdate(0)
		e.ConfigQueue = []*stnrapiv2.StunnerConfig{}
		ch <- e

		// removed configs are pushed to nobody: wait for the server to stop serving them
		Eventually(func() error { _, err := cdsc1.Load(); return err }).WithTimeout(settle).Should(HaveOccurred())

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
		config        *stnrapiv2.StunnerConfig
	)

	BeforeAll(func() {
		zc := zap.NewProductionConfig()
		zc.Level = zap.NewAtomicLevelAt(testerLogLevel)
		z, err := zc.Build()
		Expect(err).To(Succeed(), "loggerFactory created")
		zlogger := zapr.NewLogger(z)
		log = zlogger.WithName("tester")

		// the CDS client expands environment variables: keep the placeholders
		GinkgoT().Setenv(stnrapiv2.DefaultEnvVarAddr, opdefault.DefaultSTUNnerAddressEnvVarName)
		GinkgoT().Setenv(stnrapiv2.DefaultEnvVarNodeAddr, opdefault.NodeAddressVar)

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

		config = &stnrapiv2.StunnerConfig{
			ApiVersion: stnrapiv2.ApiVersion,
			Admin: stnrapiv2.AdminConfig{
				Name:     "ns/gw1",
				LogLevel: stunnerTestLoglevel,
			},
			Auths: []stnrapiv2.AuthConfig{{
				Credentials: map[string]string{
					"username": "user",
					"password": "pass",
				},
			}},
			Listeners: []stnrapiv2.ListenerConfig{{
				Name:     "default-listener",
				Protocol: "UDP",
				Port:     stnrapiv2.DefaultPort,
				Servers:  []string{"default-server"},
			}},
			Servers: []stnrapiv2.ServerConfig{{
				Name:     "default-server",
				Type:     "TURN",
				Clusters: []string{"default-cluster"},
			}},
			// the pod address placeholder: the node patcher rewrites only the node address placeholder
			Clusters: []stnrapiv2.ClusterConfig{{
				Name:     "default-cluster",
				Protocol: "UDP",
				Addrs:    []string{opdefault.DefaultSTUNnerAddressEnvVarName},
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

	It("should expand the node address to the node's external IP", func() {
		// first client uses testnode1
		log.Info("creating CDS client instance 1", "address", addr1, "id", id1)
		cdsc1, err := cdsclient.New(addr1, id1, map[string]string{stnrapiv2.DefaultCDSNodeLabel: "testnode1"}, loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		log.Info("load default config -> no patch")
		config.Clusters[0].Addrs = []string{opdefault.DefaultSTUNnerAddressEnvVarName}
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
			return len(c.Clusters) == 1 && len(c.Clusters[0].Addrs) == 1 && c.Clusters[0].Addrs[0] == opdefault.DefaultSTUNnerAddressEnvVarName
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

		log.Info("load config that requires node name patching -> patched with testnode1 external IP")
		config.Clusters[0].Addrs = []string{opdefault.NodeAddressVar}
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
			return len(c.Clusters) == 1 && len(c.Clusters[0].Addrs) == 1 && c.Clusters[0].Addrs[0] == "1.2.3.4" // testnode 1 external ip
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

	})

	It("should expand the node address to the node's external DNS name", func() {
		// second client uses testnode2 -> external DNS!
		log.Info("creating CDS client instance 2", "address", addr1, "id", id1)
		cdsc1, err := cdsclient.New(addr1, id1, map[string]string{stnrapiv2.DefaultCDSNodeLabel: "testnode2"}, loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		log.Info("load default config -> no patch")
		config.Clusters[0].Addrs = []string{opdefault.DefaultSTUNnerAddressEnvVarName}
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
			return len(c.Clusters) == 1 && len(c.Clusters[0].Addrs) == 1 && c.Clusters[0].Addrs[0] == opdefault.DefaultSTUNnerAddressEnvVarName
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

		log.Info("load config that requires node name patching -> patched with testnode2 external DNS")
		config.Clusters[0].Addrs = []string{opdefault.NodeAddressVar}
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
			return len(c.Clusters) == 1 && len(c.Clusters[0].Addrs) == 1 && net.ParseIP(c.Clusters[0].Addrs[0]) != nil // testnode2 addr should parse as ip
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

	})

	It("should leave the placeholder alone for an unknown node", func() {
		// third client uses unknown node -> no patching!
		log.Info("creating CDS client instance 3", "address", addr1, "id", id1)
		cdsc1, err := cdsclient.New(addr1, id1, map[string]string{stnrapiv2.DefaultCDSNodeLabel: "dummy-node"}, loggerFactory)
		Expect(err).To(Succeed(), "cds client setup")

		log.Info("load default config -> no patch")
		config.Clusters[0].Addrs = []string{opdefault.DefaultSTUNnerAddressEnvVarName}
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
			return len(c.Clusters) == 1 && len(c.Clusters[0].Addrs) == 1 && c.Clusters[0].Addrs[0] == opdefault.DefaultSTUNnerAddressEnvVarName
		}, time.Second, 10*time.Millisecond).Should(BeTrue())

		log.Info("load config that requires node name patching -> should not be patched as node does not exist")
		config.Clusters[0].Addrs = []string{opdefault.NodeAddressVar}
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
			return len(c.Clusters) == 1 && len(c.Clusters[0].Addrs) == 1 && c.Clusters[0].Addrs[0] == opdefault.NodeAddressVar // left for the pod
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
func watchConfig(ch chan *stnrapiv2.StunnerConfig, d time.Duration) *stnrapiv2.StunnerConfig {
	select {
	case c := <-ch:
		_ = c.Validate() // the client does not validate, compare only validated configs
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

func zeroConfig(namespace, name, realm string) *stnrapiv2.StunnerConfig {
	id := fmt.Sprintf("%s/%s", namespace, name)
	c := cdsclient.ZeroConfig(id)
	c.Auths[0].Realm = realm
	_ = c.Validate()
	return c
}
