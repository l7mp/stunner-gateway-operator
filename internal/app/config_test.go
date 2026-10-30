package app

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "app")
}

func envOf(m map[string]string) LookupEnv {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

var _ = Describe("Config", func() {
	Context("When nothing is set", func() {
		It("should take the defaults", func() {
			cfg := NewConfig()
			cfg.BindFlags(flag.NewFlagSet("t", flag.ContinueOnError), envOf(nil))
			Expect(cfg.Complete(envOf(nil))).To(Succeed())
			Expect(cfg.CDSAdvertisedAddress).To(Equal(":13478"),
				"without the address env var the pods get the listen address")
			Expect(cfg.LeaderElection).To(BeFalse())
			Expect(cfg.LeaderElectionID).NotTo(BeEmpty())
		})
	})

	Context("When the operator environment variables are set", func() {
		var env LookupEnv

		BeforeEach(func() {
			env = envOf(map[string]string{
				EnvVarControllerName: "example.com/op",
				EnvVarAddress:        "10.0.0.5",
				EnvVarLabelFilter:    " a , ,b",
				EnvVarCustomerKey:    "key",
				EnvVarPprofAddr:      ":6060",
			})
		})

		It("should use them as the flag defaults", func() {
			cfg := NewConfig()
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			cfg.BindFlags(fs, env)
			Expect(fs.Parse([]string{"--config-discovery-address=:1234"})).To(Succeed())
			Expect(cfg.Complete(env)).To(Succeed())
			Expect(cfg.ControllerName).To(Equal("example.com/op"), "the env var is the flag default")
			Expect(cfg.CDSAdvertisedAddress).To(Equal("10.0.0.5:1234"), "env host, configured port")
			Expect(cfg.LabelFilter).To(Equal([]string{"a", "b"}))
			Expect(cfg.CustomerKey).To(Equal("key"))
			Expect(cfg.PprofAddr).To(Equal(":6060"))
		})

		It("should let an explicit flag win over the environment", func() {
			cfg := NewConfig()
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			cfg.BindFlags(fs, env)
			Expect(fs.Parse([]string{"--controller-name=example.com/flag", "--leader-elect"})).To(Succeed())
			Expect(cfg.Complete(env)).To(Succeed())
			Expect(cfg.ControllerName).To(Equal("example.com/flag"))
			Expect(cfg.LeaderElection).To(BeTrue())
		})
	})

	Context("When every flag is set", func() {
		It("should take each of them", func() {
			cfg := NewConfig()
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			cfg.BindFlags(fs, envOf(nil))
			Expect(fs.Parse([]string{
				"--controller-name=example.com/flag",
				"--throttle-timeout=3s",
				"--startup-render-timeout=7s",
				"--config-discovery-address=:2345",
				"--metrics-bind-address=:9080",
				"--health-probe-bind-address=:9081",
				"--pprof-bind-address=:9082",
				"--leader-elect",
				"--leader-election-id=example-lease",
				"--leader-election-namespace=example-ns",
			})).To(Succeed())
			Expect(cfg.Complete(envOf(map[string]string{EnvVarPprofAddr: ":6060"}))).To(Succeed())
			Expect(cfg.ControllerName).To(Equal("example.com/flag"))
			Expect(cfg.ThrottleTimeout.String()).To(Equal("3s"))
			Expect(cfg.StartupRenderTimeout.String()).To(Equal("7s"))
			Expect(cfg.CDSAddress).To(Equal(":2345"))
			Expect(cfg.CDSAdvertisedAddress).To(Equal(":2345"))
			Expect(cfg.MetricsAddr).To(Equal(":9080"))
			Expect(cfg.ProbeAddr).To(Equal(":9081"))
			Expect(cfg.PprofAddr).To(Equal(":9082"), "an explicit pprof flag wins over the env var")
			Expect(cfg.LeaderElection).To(BeTrue())
			Expect(cfg.LeaderElectionID).To(Equal("example-lease"))
			Expect(cfg.LeaderElectionNamespace).To(Equal("example-ns"))
			Expect(cfg.Summary()).NotTo(BeEmpty())
		})

		It("should enable the finalizer", func() {
			cfg := NewConfig()
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			cfg.BindFlags(fs, envOf(nil))
			Expect(fs.Parse([]string{"--enable-finalizer"})).To(Succeed())
			Expect(cfg.Complete(envOf(nil))).To(Succeed())
			Expect(cfg.EnableFinalizer).To(BeTrue())
		})
	})

	Context("When the configuration contradicts itself", func() {
		It("should reject the finalizer together with leader election", func() {
			cfg := NewConfig()
			cfg.LeaderElection = true
			cfg.EnableFinalizer = true
			Expect(cfg.Complete(envOf(nil))).NotTo(Succeed())
		})

		It("should reject a CDS address without a port", func() {
			cfg := NewConfig()
			cfg.CDSAddress = "no-port"
			Expect(cfg.Complete(envOf(nil))).NotTo(Succeed())
		})
	})
})
