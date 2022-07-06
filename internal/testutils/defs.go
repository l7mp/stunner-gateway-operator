package testutils

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var (
	TestNs            = gatewayv1alpha2.Namespace("testnamespace")
	TestStunnerConfig = "stunner-config"
	TestRealm         = "testrealm"
	TestAuthType      = "plaintext"
	TestUsername      = "testuser"
	TestPassword      = "testpass"
	TestLogLevel      = "testloglevel"
	TestMinPort       = int32(1)
	TestMaxPort       = int32(2)
	TestSectionName   = gatewayv1alpha2.SectionName("gateway-1-listener-udp")
)

// GatewayClass
var TestGwClass = gatewayv1alpha2.GatewayClass{
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
			Namespace: &TestNs,
		},
	},
}

// GatewayConfig
var TestGwConfig = stunnerv1alpha1.GatewayConfig{
	TypeMeta: metav1.TypeMeta{
		APIVersion: fmt.Sprintf("%s/%s", stunnerv1alpha1.GroupVersion.Group,
			stunnerv1alpha1.GroupVersion.Version),
		Kind: "GatewaylClass",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gatewayconfig-ok",
		Namespace: "testnamespace",
	},
	Spec: stunnerv1alpha1.GatewayConfigSpec{
		StunnerConfig: &TestStunnerConfig,
		Realm:         &TestRealm,
		AuthType:      &TestAuthType,
		Username:      &TestUsername,
		Password:      &TestPassword,
		LogLevel:      &TestLogLevel,
		MinPort:       &TestMinPort,
		MaxPort:       &TestMaxPort,
	},
}

// Gateway
var TestGw = gatewayv1alpha2.Gateway{
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
var TestUDPRoute = gatewayv1alpha2.UDPRoute{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "udproute-ok",
		Namespace: "testnamespace",
	},
	Spec: gatewayv1alpha2.UDPRouteSpec{
		CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{
			ParentRefs: []gatewayv1alpha2.ParentRef{{
				Name:        "gateway-1",
				SectionName: &TestSectionName,
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
var TestSvc = corev1.Service{
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
			Ingress: []corev1.LoadBalancerIngress{{
				IP: "1.2.3.4",
				Ports: []corev1.PortStatus{{
					Port:     1,
					Protocol: corev1.ProtocolUDP,
				}},
			}, {
				IP: "5.6.7.8",
				Ports: []corev1.PortStatus{{
					Port:     2,
					Protocol: corev1.ProtocolTCP,
				}},
			}},
		}},
}

// Node
var TestNode = corev1.Node{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "testnode-ok",
	},
	Spec: corev1.NodeSpec{},
	Status: corev1.NodeStatus{
		Addresses: []corev1.NodeAddress{{
			Type:    corev1.NodeInternalIP,
			Address: "255.255.255.255",
		}, {
			Type:    corev1.NodeExternalIP,
			Address: "1.2.3.4",
		}},
	},
}

// Endpoints for the TestSvc
var TestEndpoint = corev1.Endpoints{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "testservice-ok", // must be the same as the service!
	},
	Subsets: []corev1.EndpointSubset{{
		Addresses: []corev1.EndpointAddress{{
			IP: "1.2.3.4",
		}, {
			IP: "1.2.3.5",
		}},
		NotReadyAddresses: []corev1.EndpointAddress{{
			IP: "1.2.3.6",
		}},
		Ports: []corev1.EndpointPort{},
	}, {
		Addresses: []corev1.EndpointAddress{{
			IP: "1.2.3.7",
		}},
	}},
}
