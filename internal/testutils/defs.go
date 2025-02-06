package testutils

import (
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

var (
	TestTrue             = true
	TestFalse            = false
	TestNsName           = gwapiv1.Namespace("testnamespace")
	TestRealm            = "testrealm"
	TestAuthType         = "static"
	TestUsername         = "testuser"
	TestPassword         = "testpass"
	TestLogLevel         = "testloglevel"
	TestLabelName        = "testlabel"
	TestLabelValue       = "testvalue"
	TestSectionName      = gwapiv1.SectionName("gateway-1-listener-udp")
	TestCert64           = "dGVzdGNlcnQ=" // "testcert"
	TestKey64            = "dGVzdGtleQ==" // "testkey"
	TestReplicas         = int32(3)
	TestTerminationGrace = int64(60)
	TestImagePullPolicy  = corev1.PullAlways
	TestCPURequest       = resource.MustParse("250m")
	TestMemoryLimit      = resource.MustParse("10M")
	TestResourceRequest  = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: TestCPURequest,
	})
	TestResourceLimit = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceMemory: TestMemoryLimit,
	})
	TestResourceRequirements = corev1.ResourceRequirements{
		Limits:   TestResourceLimit,
		Requests: TestResourceRequest,
	}
	TestPort        = gwapiv1.PortNumber(1)
	TestEndPort     = gwapiv1.PortNumber(2)
	TestPortUDPName = "udp-port"
	TestProtocolUDP = corev1.ProtocolUDP
	TestPortTCPName = "tcp-port"
	TestProtocolTCP = corev1.ProtocolTCP
)

// Namespace
var TestNs = corev1.Namespace{
	ObjectMeta: metav1.ObjectMeta{
		Name:   string(TestNsName),
		Labels: map[string]string{TestLabelName: TestLabelValue},
	},
}

// GatewayClass
var TestGwClass = gwapiv1.GatewayClass{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gatewayclass-ok",
		Namespace: "testnamespace",
	},
	Spec: gwapiv1.GatewayClassSpec{
		ControllerName: gwapiv1.GatewayController(opdefault.DefaultControllerName),
		ParametersRef: &gwapiv1.ParametersReference{
			Group:     gwapiv1.Group(stnrgwv1.GroupVersion.Group),
			Kind:      gwapiv1.Kind("GatewayConfig"),
			Name:      "gatewayconfig-ok",
			Namespace: &TestNsName,
		},
	},
}

// GatewayConfig
var TestGwConfig = stnrgwv1.GatewayConfig{
	TypeMeta: metav1.TypeMeta{
		APIVersion: fmt.Sprintf("%s/%s", stnrgwv1.GroupVersion.Group,
			stnrgwv1.GroupVersion.Version),
		Kind: "GatewaylClass",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gatewayconfig-ok",
		Namespace: "testnamespace",
	},
	Spec: stnrgwv1.GatewayConfigSpec{
		Realm:    &TestRealm,
		AuthType: &TestAuthType,
		Username: &TestUsername,
		Password: &TestPassword,
		LogLevel: &TestLogLevel,
	},
}

// Gateway
var TestGw = gwapiv1.Gateway{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gateway-1",
		Namespace: "testnamespace",
		Labels:    map[string]string{"dummy-label": "dummy-value"},
	},
	Spec: gwapiv1.GatewaySpec{
		GatewayClassName: "gatewayclass-ok",
		Listeners: []gwapiv1.Listener{{
			Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
			Port:     gwapiv1.PortNumber(1),
			Protocol: gwapiv1.ProtocolType("TURN-UDP"),
		}, {
			Name:     gwapiv1.SectionName("gateway-1-listener-tcp"),
			Port:     gwapiv1.PortNumber(2),
			Protocol: gwapiv1.ProtocolType("TURN-TCP"),
		}},
	},
}

// UDPRoute
var TestUDPRoute = stnrgwv1.UDPRoute{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "udproute-ok",
		Namespace: "testnamespace",
	},
	Spec: stnrgwv1.UDPRouteSpec{
		CommonRouteSpec: gwapiv1.CommonRouteSpec{
			ParentRefs: []gwapiv1.ParentReference{{
				Name:        "gateway-1",
				SectionName: &TestSectionName,
			}},
		},
		Rules: []stnrgwv1.UDPRouteRule{{
			BackendRefs: []stnrgwv1.BackendRef{{
				BackendObjectReference: stnrgwv1.BackendObjectReference{
					Name: gwapiv1.ObjectName("testservice-ok"),
					// Port:    &TestPort,
					// EndPort: &TestEndPort,
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
			opdefault.RelatedGatewayKey: "testnamespace/gateway-1",
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
				// NodePort: 30001,
			},
			{
				Name:     "tcp-ok",
				Protocol: corev1.ProtocolTCP,
				Port:     2,
				// NodePort: 30002,
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
				}, {
					Port:     2,
					Protocol: corev1.ProtocolTCP,
				}},
			}},
		},
	},
}

// Node
var TestNode = corev1.Node{
	ObjectMeta: metav1.ObjectMeta{
		Name: "testnode-ok",
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

// EndpointSlice for the TestSvc
var TestEndpointSlice = discoveryv1.EndpointSlice{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "testendpointslice-ok",
		Labels: map[string]string{ // bound to the svc by a label
			"kubernetes.io/service-name": "testservice-ok",
		},
	},
	AddressType: discoveryv1.AddressTypeIPv4,
	Endpoints: []discoveryv1.Endpoint{
		{
			Addresses: []string{
				"1.2.3.4",
				"1.2.3.5",
			},
			Conditions: discoveryv1.EndpointConditions{
				Ready:       &TestTrue,
				Serving:     &TestTrue,
				Terminating: &TestFalse,
			},
		},
		{
			Addresses: []string{
				"1.2.3.6",
			},
			Conditions: discoveryv1.EndpointConditions{
				Ready:       &TestFalse,
				Serving:     &TestFalse,
				Terminating: &TestTrue,
			},
		},
		{
			Addresses: []string{
				"1.2.3.7",
			},
			Conditions: discoveryv1.EndpointConditions{
				Ready:       &TestFalse,
				Serving:     &TestTrue,
				Terminating: &TestFalse,
			},
		},
	},
	Ports: []discoveryv1.EndpointPort{{
		Name:     &TestPortUDPName,
		Protocol: &TestProtocolUDP,
	}, {
		Name:     &TestPortTCPName,
		Protocol: &TestProtocolTCP,
	}},
}

// TestSecret for TLS tests
var TestSecret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "testsecret-ok",
	},
	Type: corev1.SecretTypeTLS,
	Data: map[string][]byte{
		"tls.crt": []byte("testcert"),
		"tls.key": []byte("testkey"),
	},
}

// TestAuthSecret for external auth tests
var TestAuthSecret = corev1.Secret{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "testauthsecret-ok",
	},
	Type: corev1.SecretTypeOpaque,
	Data: map[string][]byte{
		"username": []byte("ext-testuser"),
		"password": []byte("ext-testpass"),
		"secret":   []byte("ext-secret"),
	},
}

// StaticService
var TestStaticSvc = stnrgwv1.StaticService{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "teststaticservice-ok",
	},
	Spec: stnrgwv1.StaticServiceSpec{
		Prefixes: []string{"10.11.12.13", "10.11.12.14", "10.11.12.15"},
	},
}

// Dataplane
var TestDataplane = stnrgwv1.Dataplane{
	ObjectMeta: metav1.ObjectMeta{
		Name: opdefault.DefaultDataplaneName,
	},
	Spec: stnrgwv1.DataplaneSpec{
		Replicas:                      &TestReplicas,
		Image:                         "testimage-1",
		Command:                       []string{"testcommand-1"},
		Args:                          []string{"arg-1", "arg-2"},
		ImagePullPolicy:               &TestImagePullPolicy,
		TerminationGracePeriodSeconds: &TestTerminationGrace,
		Resources:                     &TestResourceRequirements,
		HostNetwork:                   true,
		Affinity:                      nil,
	},
}

// For backward compatibility
var TestUDPRouteV1A2 = gwapiv1a2.UDPRoute{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "udproute-ok",
		Namespace: "testnamespace",
	},
	Spec: gwapiv1a2.UDPRouteSpec{
		CommonRouteSpec: gwapiv1.CommonRouteSpec{
			ParentRefs: []gwapiv1.ParentReference{{
				Name:        "gateway-1",
				SectionName: &TestSectionName,
			}},
		},
		Rules: []gwapiv1a2.UDPRouteRule{{
			BackendRefs: []gwapiv1a2.BackendRef{{
				BackendObjectReference: gwapiv1a2.BackendObjectReference{
					Name: gwapiv1.ObjectName("testservice-ok"),
					// port is mandatory
					Port: &TestPort,
				},
			}},
		}},
	},
}

// for testing with DaemonSets
var TestDaemonSet = appv1.DaemonSet{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gateway-1",
		Namespace: "testnamespace",
	},
	Spec: appv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": "stunner"},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "stunner"},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "stunnerd",
						Image: "l7mp/stunnerd:latest",
					},
				},
			},
		},
	},
}
