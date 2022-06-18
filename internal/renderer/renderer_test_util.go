package renderer

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	// "github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var (
	testNs            = gatewayv1alpha2.Namespace("testnamespace")
	testStunnerConfig = "stunner-config"
	testRealm         = "testrealm"
	testAuthType      = "plaintext"
	testUsername      = "testuser"
	testPassword      = "testpass"
	testLogLevel      = "testloglevel"
	testMinport       = int32(1)
	testMaxPort       = int32(2)
)

// GatewayClass
var gwClass = gatewayv1alpha2.GatewayClass{
	// ApiVersion: gatewayv1alpha2.GroupName,
	// Kind:       "GatewayClass",
	ObjectMeta: metav1.ObjectMeta{
		Name: "gatewayclass-ok",
	},
	Spec: gatewayv1alpha2.GatewayClassSpec{
		ControllerName: gatewayv1alpha2.GatewayController(operator.DefaultControllerName),
		ParametersRef: &gatewayv1alpha2.ParametersReference{
			Group:     gatewayv1alpha2.Group(stunnerv1alpha1.GroupVersion.Group),
			Kind:      gatewayv1alpha2.Kind("GatewayConfig"),
			Name:      "gatewayconfig-ok",
			Namespace: &testNs,
		},
	},
	Status: gatewayv1alpha2.GatewayClassStatus{},
}

// GatewayConfig
var gwConfig = stunnerv1alpha1.GatewayConfig{
	// ApiVersion: stunnerv1alpha1.GroupName,
	// Kind:       "GatewayConfig",
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gatewayconfig-ok",
		Namespace: "testnamespace",
	},
	Spec: stunnerv1alpha1.GatewayConfigSpec{
		StunnerConfig: &testStunnerConfig,
		Realm:         &testRealm,
		AuthType:      &testAuthType,
		Username:      &testUsername,
		Password:      &testPassword,
		LogLevel:      &testLogLevel,
		MinPort:       &testMinport,
		MaxPort:       &testMaxPort,
	},
	// Status: stunnerv1alpha1.GatewayConfigStatus{},
}

////////////////////////////
type renderTestConfig struct {
	name   string
	cls    []gatewayv1alpha2.GatewayClass
	cfs    []stunnerv1alpha1.GatewayConfig
	gws    []gatewayv1alpha2.Gateway
	rs     []gatewayv1alpha2.UDPRoute
	svcs   []corev1.Service
	prep   func(c *renderTestConfig)
	tester func(t *testing.T, r *Renderer)
}

// start with default config and then reconcile with the given config
func renderTester(t *testing.T, testConf []renderTestConfig) {
	log := zap.New()

	for _, c := range testConf {
		t.Run(c.name, func(t *testing.T) {
			log.V(1).Info(fmt.Sprintf("-------------- Running test: %s -------------", c.name))

			c.prep(&c)

			log.V(1).Info("setting up config renderer")
			r := NewRenderer(RendererConfig{
				Logger: log.WithName("renderer"),
			})

			log.V(1).Info("setting up operator")
			op := operator.NewOperator(operator.OperatorConfig{
				ControllerName: operator.DefaultControllerName,
				RenderCh:       r.GetRenderChannel(),
				Logger:         log,
			})
			r.SetOperator(op)

			log.V(1).Info("preparing local storage")
			op.SetupStore()
			for _, o := range c.cls {
				op.AddGatewayClass(&o)
			}
			for _, o := range c.cfs {
				op.AddGatewayConfig(&o)
			}
			for _, o := range c.gws {
				op.AddGateway(&o)
			}
			for _, o := range c.rs {
				op.AddUDPRoute(&o)
			}
			for _, o := range c.svcs {
				op.AddService(&o)
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
