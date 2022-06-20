package renderer

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/operator"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

// for debugging
var testerLogLevel = zapcore.Level(-4)

// info
//var testerLogLevel = zapcore.DebugLevel
//var testerLogLevel = zapcore.ErrorLevel

var (
	testNs            = gatewayv1alpha2.Namespace("testnamespace")
	testStunnerConfig = "stunner-config"
	testRealm         = "testrealm"
	testAuthType      = "plaintext"
	testUsername      = "testuser"
	testPassword      = "testpass"
	testLogLevel      = "testloglevel"
	testMinPort       = int32(1)
	testMaxPort       = int32(2)
	testSectionName   = gatewayv1alpha2.SectionName("gateway-1-listener-udp")
)

// GatewayClass
var testGwClass = gatewayv1alpha2.GatewayClass{
	// TypeMeta: metav1.TypeMeta{
	// 	APIVersion: fmt.Sprintf("%s/%s", gatewayv1alpha2.GroupVersion.Group, gatewayv1alpha2.GroupVersion.Version),
	// 	Kind:       "GatewaylClass",
	// },
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gatewayclass-ok",
		Namespace: "testnamespace",
	},
	Spec: gatewayv1alpha2.GatewayClassSpec{
		ControllerName: gatewayv1alpha2.GatewayController(config.DefaultControllerName),
		ParametersRef: &gatewayv1alpha2.ParametersReference{
			Group:     gatewayv1alpha2.Group(stunnerv1alpha1.GroupVersion.Group),
			Kind:      gatewayv1alpha2.Kind("GatewayConfig"),
			Name:      "gatewayconfig-ok",
			Namespace: &testNs,
		},
	},
}

// GatewayConfig
var testGwConfig = stunnerv1alpha1.GatewayConfig{
	TypeMeta: metav1.TypeMeta{
		APIVersion: fmt.Sprintf("%s/%s", stunnerv1alpha1.GroupVersion.Group, stunnerv1alpha1.GroupVersion.Version),
		Kind:       "GatewaylClass",
	},
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
		MinPort:       &testMinPort,
		MaxPort:       &testMaxPort,
	},
}

// Gateway
var testGw = gatewayv1alpha2.Gateway{
	// TypeMeta: metav1.TypeMeta{
	// 	APIVersion: fmt.Sprintf("%s/%s", gatewayv1alpha2.GroupVersion.Group, gatewayv1alpha2.GroupVersion.Version),
	// 	Kind:       "Gateway",
	// },
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gateway-1",
		Namespace: "testnamespace",
	},
	Spec: gatewayv1alpha2.GatewaySpec{
		GatewayClassName: "gatewayclass-ok",
		Listeners: []gatewayv1alpha2.Listener{{
			Name:     gatewayv1alpha2.SectionName("gateway-1-listener-udp"),
			Port:     gatewayv1alpha2.PortNumber(1),
			Protocol: gatewayv1alpha2.ProtocolType("UDP"),
		}, {
			Name:     gatewayv1alpha2.SectionName("invalid"),
			Port:     gatewayv1alpha2.PortNumber(3),
			Protocol: gatewayv1alpha2.ProtocolType("dummy"),
		}, {
			Name:     gatewayv1alpha2.SectionName("gateway-1-listener-tcp"),
			Port:     gatewayv1alpha2.PortNumber(2),
			Protocol: gatewayv1alpha2.ProtocolType("TCP"),
		}},
	},
}

// UDPRoute
var testUDPRoute = gatewayv1alpha2.UDPRoute{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "udproute-ok",
		Namespace: "testnamespace",
	},
	Spec: gatewayv1alpha2.UDPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{
			ParentRefs: []gatewayv1alpha2.ParentRef{{
				Name:        "gateway-1",
				SectionName: &testSectionName,
			}},
		},
		Rules: []gatewayv1alpha2.UDPRouteRule{{
			BackendRefs: []gatewayv1alpha2.BackendRef{{
				BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
					Name: gatewayv1alpha2.ObjectName("testservice-ok"),
				},
			}},
		}},
	},
}

// Service
var testSvc = corev1.Service{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "testservice-ok",
		Annotations: map[string]string{
			config.GatewayAddressAnnotationKey: "testnamespace/gateway-1",
		},
	},
	Spec: corev1.ServiceSpec{
		Type:     corev1.ServiceTypeLoadBalancer,
		Selector: map[string]string{"app": "dummy"},
		Ports: []corev1.ServicePort{
			{
				Name:     "udp-ok",
				Protocol: corev1.ProtocolUDP,
				Port:     1,
			},
		},
	},
	Status: corev1.ServiceStatus{
		LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{IP: "1.2.3.4"}, {IP: "5.6.7.8"}},
		}},
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
				Logger: log.WithName("renderer"),
			})

			log.V(1).Info("setting up operator")
			op := operator.NewOperator(operator.OperatorConfig{
				ControllerName: config.DefaultControllerName,
				RenderCh:       r.GetRenderChannel(),
				Logger:         log,
			})
			r.SetOperator(op)

			log.V(1).Info("preparing local storage")
			op.SetupStore()
			for i := range c.cls {
				op.AddGatewayClass(&c.cls[i])
			}
			for i := range c.cfs {
				op.AddGatewayConfig(&c.cfs[i])
			}
			for i := range c.gws {
				// fsck you Go!!!!!!1
				op.AddGateway(&c.gws[i])
			}
			for i := range c.rs {
				op.AddUDPRoute(&c.rs[i])
			}
			for i := range c.svcs {
				op.AddService(&c.svcs[i])
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
