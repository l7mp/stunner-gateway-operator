package testutils

import (
	"fmt"

	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

var (
	TestTrue                  = true
	TestNsName                = gwapiv1a2.Namespace("testnamespace")
	TestStunnerConfig         = "stunner-config"
	TestRealm                 = "testrealm"
	TestMetricsEndpoint       = "testmetrics"
	TestHealthCheckEndpoint   = "testhealth"
	TestAuthType              = "plaintext"
	TestUsername              = "testuser"
	TestPassword              = "testpass"
	TestLogLevel              = "testloglevel"
	TestMinPort               = int32(1)
	TestMaxPort               = int32(2)
	TestLabelName             = "testlabel"
	TestLabelValue            = "testvalue"
	TestSectionName           = gwapiv1a2.SectionName("gateway-1-listener-udp")
	TestCert64                = "dGVzdGNlcnQ=" // "testcert"
	TestKey64                 = "dGVzdGtleQ==" // "testkey"
	TestReplicas              = int32(3)
	TestDeployStrategy        = appv1.DeploymentStrategy{Type: appv1.RecreateDeploymentStrategyType, RollingUpdate: nil}
	TestFieldSelector         = corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"}
	TestEnvEnvVarSource       = corev1.EnvVarSource{FieldRef: &TestFieldSelector}
	TestConfigMapVolumeSource = corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "testgateway-1"}, Optional: &TestTrue}
	TestTerminationGrace      = int64(60)
	TestProbe                 = corev1.Probe{PeriodSeconds: 15, SuccessThreshold: 2, FailureThreshold: 3}
	TestCPURequest            = resource.NewQuantity(100, resource.DecimalSI)
	TestMemoryLimit           = resource.NewQuantity(500, resource.DecimalSI)
	TestResourceRequest       = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceRequestsCPU: *TestCPURequest,
	})
	TestResourceLimit = corev1.ResourceList(map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceLimitsMemory: *TestMemoryLimit,
	})
)

// Namespace
var TestNs = corev1.Namespace{
	ObjectMeta: metav1.ObjectMeta{
		Name:   string(TestNsName),
		Labels: map[string]string{TestLabelName: TestLabelValue},
	},
}

// GatewayClass
var TestGwClass = gwapiv1a2.GatewayClass{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gatewayclass-ok",
		Namespace: "testnamespace",
	},
	Spec: gwapiv1a2.GatewayClassSpec{
		ControllerName: gwapiv1a2.GatewayController(opdefault.DefaultControllerName),
		ParametersRef: &gwapiv1a2.ParametersReference{
			Group:     gwapiv1a2.Group(stnrv1a1.GroupVersion.Group),
			Kind:      gwapiv1a2.Kind("GatewayConfig"),
			Name:      "gatewayconfig-ok",
			Namespace: &TestNsName,
		},
	},
}

// GatewayConfig
var TestGwConfig = stnrv1a1.GatewayConfig{
	TypeMeta: metav1.TypeMeta{
		APIVersion: fmt.Sprintf("%s/%s", stnrv1a1.GroupVersion.Group,
			stnrv1a1.GroupVersion.Version),
		Kind: "GatewaylClass",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gatewayconfig-ok",
		Namespace: "testnamespace",
	},
	Spec: stnrv1a1.GatewayConfigSpec{
		StunnerConfig:       &TestStunnerConfig,
		Realm:               &TestRealm,
		MetricsEndpoint:     &TestMetricsEndpoint,
		HealthCheckEndpoint: &TestHealthCheckEndpoint,
		AuthType:            &TestAuthType,
		Username:            &TestUsername,
		Password:            &TestPassword,
		LogLevel:            &TestLogLevel,
		MinPort:             &TestMinPort,
		MaxPort:             &TestMaxPort,
	},
}

// Gateway
var TestGw = gwapiv1a2.Gateway{
	// TypeMeta: metav1.TypeMeta{
	// 	APIVersion: fmt.Sprintf("%s/%s", gwapiv1a2.GroupVersion.Group, gwapiv1a2.GroupVersion.Version),
	// 	Kind:       "Gateway",
	// },
	ObjectMeta: metav1.ObjectMeta{
		Name:      "gateway-1",
		Namespace: "testnamespace",
		Labels:    map[string]string{"dummy-label": "dummy-value"},
	},
	Spec: gwapiv1a2.GatewaySpec{
		GatewayClassName: "gatewayclass-ok",
		Listeners: []gwapiv1a2.Listener{{
			Name:     gwapiv1a2.SectionName("gateway-1-listener-udp"),
			Port:     gwapiv1a2.PortNumber(1),
			Protocol: gwapiv1a2.ProtocolType("UDP"),
		}, {
			Name:     gwapiv1a2.SectionName("invalid"),
			Port:     gwapiv1a2.PortNumber(3),
			Protocol: gwapiv1a2.ProtocolType("dummy"),
		}, {
			Name:     gwapiv1a2.SectionName("gateway-1-listener-tcp"),
			Port:     gwapiv1a2.PortNumber(2),
			Protocol: gwapiv1a2.ProtocolType("TCP"),
		}},
	},
}

// UDPRoute
var TestUDPRoute = gwapiv1a2.UDPRoute{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "udproute-ok",
		Namespace: "testnamespace",
	},
	Spec: gwapiv1a2.UDPRouteSpec{
		CommonRouteSpec: gwapiv1a2.CommonRouteSpec{
			ParentRefs: []gwapiv1a2.ParentReference{{
				Name:        "gateway-1",
				SectionName: &TestSectionName,
			}},
		},
		Rules: []gwapiv1a2.UDPRouteRule{{
			BackendRefs: []gwapiv1a2.BackendRef{{
				BackendObjectReference: gwapiv1a2.BackendObjectReference{
					Name: gwapiv1a2.ObjectName("testservice-ok"),
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
var TestStaticSvc = stnrv1a1.StaticService{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "testnamespace",
		Name:      "teststaticservice-ok",
	},
	Spec: stnrv1a1.StaticServiceSpec{
		Prefixes: []string{"10.11.12.13", "10.11.12.14", "10.11.12.15"},
	},
}

// Dataplane
var TestDataplane = stnrv1a1.Dataplane{
	ObjectMeta: metav1.ObjectMeta{
		Name: opdefault.DefaultDataplaneName,
	},
	Spec: stnrv1a1.DataplaneSpec{
		Replicas: &TestReplicas,
		Strategy: &TestDeployStrategy,
		Containers: []stnrv1a1.Container{{
			Name:    "testcontainer-1",
			Image:   "testimage-1",
			Command: []string{"testcommand-1-1", "testcommand-1-2"},
			Args:    []string{"testarg-1-1", "testarg-1-2"},
			Ports: []corev1.ContainerPort{{
				Name:          "testport-1-1",
				ContainerPort: int32(1),
				Protocol:      corev1.ProtocolUDP,
			}, {
				Name:          "testport-1-2",
				ContainerPort: int32(2),
				Protocol:      corev1.ProtocolTCP,
			}},
			EnvFrom: []corev1.EnvFromSource{},
			Env: []corev1.EnvVar{{
				Name:  "TEST_ENV_1",
				Value: "test-env-val",
			}, {
				Name:      "TEST_ENV_2",
				ValueFrom: &TestEnvEnvVarSource,
			}},
			Resources: stnrv1a1.ResourceRequirements{
				Limits:   TestResourceLimit,
				Requests: TestResourceRequest,
			},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "testvolume-name",
				ReadOnly:  true,
				MountPath: "/tmp/mount",
			}},
			LivenessProbe:   &TestProbe,
			ReadinessProbe:  &TestProbe,
			ImagePullPolicy: corev1.PullAlways,
			SecurityContext: nil,
		}, {
			Name:    "testcontainer-2",
			Image:   "testimage-2",
			Command: []string{"testcommand-2-1", "testcommand-2-2"},
			Args:    []string{"testarg-2-1", "testarg-2-2"},
			Ports: []corev1.ContainerPort{{
				Name:          "testport-2-1",
				ContainerPort: int32(1),
				Protocol:      corev1.ProtocolUDP,
			}, {
				Name:          "testport-2-2",
				ContainerPort: int32(2),
				Protocol:      corev1.ProtocolTCP,
			}},
			EnvFrom: []corev1.EnvFromSource{},
			Env:     []corev1.EnvVar{},
			Resources: stnrv1a1.ResourceRequirements{
				Limits:   TestResourceLimit,
				Requests: TestResourceRequest,
			},
			LivenessProbe:   &TestProbe,
			ReadinessProbe:  &TestProbe,
			ImagePullPolicy: corev1.PullIfNotPresent,
			SecurityContext: nil,
		}},
		Volumes: []corev1.Volume{{
			Name: "testvolume-name",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &TestConfigMapVolumeSource,
			},
		}},
		TerminationGracePeriodSeconds: &TestTerminationGrace,
		HostNetwork:                   true,
		Affinity:                      nil,
	},
}
