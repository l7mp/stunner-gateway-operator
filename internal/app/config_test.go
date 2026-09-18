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
				EnvVarMode:           "legacy",
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
			Expect(cfg.DataplaneMode).To(Equal("legacy"))
			Expect(cfg.CDSAdvertisedAddress).To(Equal("10.0.0.5:1234"), "env host, configured port")
			Expect(cfg.LabelFilter).To(Equal([]string{"a", "b"}))
			Expect(cfg.CustomerKey).To(Equal("key"))
			Expect(cfg.PprofAddr).To(Equal(":6060"))
		})

		It("should let an explicit flag win over the environment", func() {
			cfg := NewConfig()
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			cfg.BindFlags(fs, env)
			Expect(fs.Parse([]string{"--dataplane-mode=managed", "--leader-elect"})).To(Succeed())
			Expect(cfg.Complete(env)).To(Succeed())
			Expect(cfg.DataplaneMode).To(Equal("managed"))
			Expect(cfg.LeaderElection).To(BeTrue())
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
