package renderer

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// "github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// var testerLogLevel = zapcore.Level(-4)
// var testerLogLevel = zapcore.DebugLevel
var testerLogLevel = zapcore.ErrorLevel

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gwapiv1a2.AddToScheme(scheme))
	utilruntime.Must(stnrv1a1.AddToScheme(scheme))
}

type renderTestConfig struct {
	name   string
	cls    []gwapiv1a2.GatewayClass
	cfs    []stnrv1a1.GatewayConfig
	gws    []gwapiv1a2.Gateway
	rs     []gwapiv1a2.UDPRoute
	svcs   []corev1.Service
	nodes  []corev1.Node
	eps    []corev1.Endpoints
	scrts  []corev1.Secret
	ascrts []corev1.Secret
	nss    []corev1.Namespace
	ssvcs  []stnrv1a1.StaticService
	prep   func(c *renderTestConfig)
	tester func(t *testing.T, r *Renderer)
}

// start with default config and then reconcile with the given config
func renderTester(t *testing.T, testConf []renderTestConfig) {
	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(testerLogLevel)
	z, err := zc.Build()
	assert.NoError(t, err, "logger created")
	log := zapr.NewLogger(z)

	for i := range testConf {
		c := testConf[i]
		t.Run(c.name, func(t *testing.T) {
			log.V(1).Info(fmt.Sprintf("-------------- Running test: %s -------------", c.name))

			c.prep(&c)

			log.V(1).Info("setting up config renderer")
			r := NewRenderer(RendererConfig{
				Scheme: scheme,
				Logger: log.WithName("renderer"),
			})

			log.V(1).Info("preparing local storage")

			store.GatewayClasses.Flush()
			for i := range c.cls {
				store.GatewayClasses.Upsert(&c.cls[i])
			}

			store.GatewayConfigs.Flush()
			for i := range c.cfs {
				store.GatewayConfigs.Upsert(&c.cfs[i])
			}

			store.Gateways.Flush()
			for i := range c.gws {
				store.Gateways.Upsert(&c.gws[i])
			}

			store.UDPRoutes.Flush()
			for i := range c.rs {
				store.UDPRoutes.Upsert(&c.rs[i])
			}

			store.Services.Flush()
			for i := range c.svcs {
				store.Services.Upsert(&c.svcs[i])
			}

			store.Nodes.Flush()
			for i := range c.nodes {
				store.Nodes.Upsert(&c.nodes[i])
			}

			store.Endpoints.Flush()
			for i := range c.eps {
				store.Endpoints.Upsert(&c.eps[i])
			}

			store.Secrets.Flush()
			for i := range c.scrts {
				store.Secrets.Upsert(&c.scrts[i])
			}

			store.AuthSecrets.Flush()
			for i := range c.ascrts {
				store.AuthSecrets.Upsert(&c.ascrts[i])
			}

			store.Namespaces.Flush()
			for i := range c.nss {
				store.Namespaces.Upsert(&c.nss[i])
			}

			store.StaticServices.Flush()
			for i := range c.ssvcs {
				store.StaticServices.Upsert(&c.ssvcs[i])
			}

			log.V(1).Info("starting renderer thread")
			ctx, cancel := context.WithCancel(context.Background())
			err := r.Start(ctx)
			assert.NoError(t, err, "renderer thread started")
			defer cancel()

			c.tester(t, r)

		})
	}
}
