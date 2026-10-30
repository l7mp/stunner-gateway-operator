package renderer

import (
	// "context"
	// "fmt"

	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"
	opdefault "github.com/l7mp/stunner-gateway-operator/v2/pkg/config"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

const defaultExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyType("")

func TestRenderServiceUtil(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "public-ip ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// update owner ref so that we accept the public IP
				s := testutils.TestSvc.DeepCopy()
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addrs, err := r.getPublicAddr(gw)
				assert.NoError(t, err, "owner ref found")
				assert.NotNil(t, addrs, "public addr-port found")
				assert.Len(t, addrs, 2, "public addr-port len")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[0].aType, "public addr type 1 ok")
				assert.Equal(t, "1.2.3.4", addrs[0].addr, "public addr 1 ok")
				assert.Equal(t, 1, addrs[0].port, "public port 1 ok")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[1].aType, "public addr type 2 ok")
				assert.Equal(t, "1.2.3.4", addrs[1].addr, "public addr 2 ok")
				assert.Equal(t, 2, addrs[1].port, "public port 2 ok")
			},
		},
		{
			name: "fallback to hostname ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				s1.Status.LoadBalancer.Ingress[0].IP = ""
				s1.Status.LoadBalancer.Ingress[0].Hostname = "dummy-hostname"
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addrs, err := r.getPublicAddr(gw)
				assert.NoError(t, err, "owner ref found")
				assert.NotNil(t, addrs, "public addr-port found")
				assert.Len(t, addrs, 2, "public addr-port len")
				assert.Equal(t, gwapiv1.HostnameAddressType, addrs[0].aType, "public addr type 1 ok")
				assert.Equal(t, "dummy-hostname", addrs[0].addr, "public addr 1 ok")
				assert.Equal(t, 1, addrs[0].port, "public port 1 ok")
				assert.Equal(t, gwapiv1.HostnameAddressType, addrs[1].aType, "public addr type 2 ok")
				assert.Equal(t, "dummy-hostname", addrs[1].addr, "public addr 2 ok")
				assert.Equal(t, 2, addrs[1].port, "public port 2 ok")
			},
		},
		{
			name: "no service-port status ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				s1.Status = corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{
							IP: "1.2.3.4",
						}},
					}}
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addrs, err := r.getPublicAddr(gw)
				assert.NoError(t, err, "owner ref found")
				assert.NotNil(t, addrs, "public addr-port found")
				assert.Len(t, addrs, 2, "public addr-port len")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[0].aType, "public addr type 1 ok")
				assert.Equal(t, "1.2.3.4", addrs[0].addr, "public addr 1 ok")
				assert.Equal(t, 1, addrs[0].port, "public port 1 ok")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[1].aType, "public addr type 2 ok")
				assert.Equal(t, "1.2.3.4", addrs[1].addr, "public addr 2 ok")
				assert.Equal(t, 2, addrs[1].port, "public port 2 ok")
			},
		},
		{
			name: "wrong annotation name errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				delete(s1.ObjectMeta.Annotations, opdefault.RelatedGatewayKey)
				s1.ObjectMeta.Annotations["dummy"] = "dummy"
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddr(gw)
				assert.Error(t, err, "public addr-port found")
				assert.Equal(t, NewNonCriticalError(PublicAddressNotFound), err,
					"public addr-port found errs")
			},
		},
		{
			name: "wrong annotation errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.ObjectMeta.Annotations[opdefault.RelatedGatewayKey] = "dummy"
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddr(gw)
				assert.Error(t, err, "public addr-port found")
				assert.Equal(t, NewNonCriticalError(PublicAddressNotFound), err,
					"public addr-port found errs")
			},
		},
		{
			name:  "no owner-ref errs",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{},
			nodes: []corev1.Node{testutils.TestNode},
			svcs:  []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				// no owner-ref
				s1.Spec.Ports[0].Protocol = corev1.ProtocolSCTP
				s1.Spec.Ports = []corev1.ServicePort{{
					Name:     "udp-ok",
					Protocol: corev1.ProtocolUDP,
					Port:     1,
					NodePort: 1234,
				}}
				s1.Status = corev1.ServiceStatus{}
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				_, err = r.getPublicAddr(gw)
				assert.Error(t, err, "owner ref not found")
				assert.Equal(t, NewNonCriticalError(PublicAddressNotFound), err,
					"public addr-port found errs")
			},
		},
		{
			name: "invalid listener proto errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// update owner ref so that we accept the public IP
				s := testutils.TestSvc.DeepCopy()
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}

				// add invalid listener
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("TURN-UDP"),
				}, {
					Name:     gwapiv1.SectionName("invalid"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("dummy"),
				}, {
					Name:     gwapiv1.SectionName("gateway-1-listener-tcp"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("TURN-TCP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addrs, err := r.getPublicAddr(gw)
				assert.NotNil(t, addrs, "public addr-port found")
				assert.Equal(t, NewNonCriticalError(PublicListenerAddressNotFound), err,
					"public listener addr-port found errs")
				assert.Len(t, addrs, 3, "public addr-port len")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[0].aType, "public addr type 1 ok")
				assert.Equal(t, "1.2.3.4", addrs[0].addr, "public addr 1 ok")
				assert.Equal(t, 1, addrs[0].port, "public port 1 ok")
				assert.Equal(t, "", string(addrs[1].aType), "public addr type 2 ok")
				assert.Equal(t, "", addrs[1].addr, "public addr 2 ok")
				assert.Equal(t, 0, addrs[1].port, "public port 2 ok")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[2].aType, "public addr type 2 ok")
				assert.Equal(t, "1.2.3.4", addrs[2].addr, "public addr 2 ok")
				assert.Equal(t, 2, addrs[2].port, "public port 2 ok")
			},
		},
		{
			name: "wrong port errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Ports[0].Port = 12
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addrs, err := r.getPublicAddr(gw)
				assert.Error(t, err, "public addr-port errs")
				assert.Equal(t, NewNonCriticalError(PublicListenerAddressNotFound), err,
					"public listener addr-port found errs")
				assert.NotNil(t, addrs, "public addr-port found")
				assert.Len(t, addrs, 2, "public addr-port len")
				assert.Equal(t, "", string(addrs[0].aType), "public addr type 1 ok")
				assert.Equal(t, "", addrs[0].addr, "public addr 1 ok")
				assert.Equal(t, 0, addrs[0].port, "public port 1 ok")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[1].aType, "public addr type 2 ok")
				assert.Equal(t, "1.2.3.4", addrs[1].addr, "public addr 2 ok")
				assert.Equal(t, 2, addrs[1].port, "public port 2 ok")
			},
		},
		{
			name: "multiple service-ports public-ip ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				s1.Status = corev1.ServiceStatus{
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
								Port:     1,
								Protocol: corev1.ProtocolUDP,
							}, {
								Port:     2,
								Protocol: corev1.ProtocolTCP,
							}},
						}},
					}}
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				addrs, err := r.getPublicAddr(gw)
				assert.NoError(t, err, "owner ref found")
				assert.NotNil(t, addrs, "public addr-port found")
				assert.Len(t, addrs, 2, "public addr-port len")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[0].aType, "public addr type 1 ok")
				assert.Equal(t, "1.2.3.4", addrs[0].addr, "public addr 1 ok")
				assert.Equal(t, 1, addrs[0].port, "public port 1 ok")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[1].aType, "public addr type 2 ok")
				assert.Equal(t, "5.6.7.8", addrs[1].addr, "public addr 2 ok")
				assert.Equal(t, 2, addrs[1].port, "public port 2 ok")
			},
		},
		{
			name:  "nodeport IP OK",
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{},
			nodes: []corev1.Node{testutils.TestNode},
			svcs:  []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s1 := testutils.TestSvc.DeepCopy()
				s1.Spec.Type = corev1.ServiceTypeNodePort
				s1.Spec.Ports[0].NodePort = 30001
				s1.Spec.Ports[1].NodePort = 30002
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				// set owner-ref so that getPublic... finds our service
				s := types.NamespacedName{
					Namespace: testutils.TestSvc.GetNamespace(),
					Name:      testutils.TestSvc.GetName(),
				}
				svc := store.Services.GetObject(s)
				assert.NoError(t, controllerutil.SetOwnerReference(gw, svc, r.scheme), "set-owner")
				store.Services.Upsert(svc)

				addrs, err := r.getPublicAddr(gw)
				assert.NoError(t, err, "owner ref found")
				assert.NotNil(t, addrs, "public addr-port found")
				assert.Len(t, addrs, 2, "public addr-port len")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[0].aType, "public addr type 1 ok")
				assert.Equal(t, "1.2.3.4", addrs[0].addr, "public addr 1 ok")
				assert.Equal(t, svc.Spec.Ports[0].NodePort, int32(addrs[0].port), "public port 1 ok")
				assert.Equal(t, gwapiv1.IPAddressType, addrs[1].aType, "public addr type 2 ok")
				assert.Equal(t, "1.2.3.4", addrs[1].addr, "public addr 2 ok")
				assert.Equal(t, svc.Spec.Ports[1].NodePort, int32(addrs[1].port), "public port 2 ok")
			},
		},
		{
			name: "lb service - single listener",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*w}

			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")
				assert.Equal(t, corev1.ServiceAffinityClientIP, spec.SessionAffinity,
					"session affinity")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp name")
				assert.Equal(t, string(gw.Spec.Listeners[0].Protocol), string(sp[0].Protocol), "sp proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp port")
			},
		},
		{
			name: "lb service - single listener, no valid listener",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("dummy"),
				}}
				c.gws = []gwapiv1.Gateway{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.Nil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
			},
		},
		{
			name: "lb service - multi-listener, single proto",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners[1].Protocol = gwapiv1.ProtocolType("UDP")
				c.gws = []gwapiv1.Gateway{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
				assert.Equal(t, string(gw.Spec.Listeners[1].Name), string(sp[1].Name), "sp 2 - name")
				assert.Equal(t, "UDP", string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[1].Port), string(sp[1].Port), "sp 2 - port")
			},
		},
		{
			name: "lb service - multi-listener-multi proto - first valid",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				w.Spec.Listeners[0].Protocol = gwapiv1.ProtocolType("dummy-2")
				c.gws = []gwapiv1.Gateway{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 1, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[1].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "TCP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[1].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - multi-listener-multi-proto - mixed proto annotation in gateway - all valid",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				mixedProtoAnnotation := map[string]string{
					opdefault.MixedProtocolAnnotationKey: "true",
				}
				w.ObjectMeta.SetAnnotations(mixedProtoAnnotation)
				c.gws = []gwapiv1.Gateway{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 2, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")
				emp, found := as[opdefault.MixedProtocolAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, opdefault.MixedProtocolAnnotationValue, emp, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto-udp")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
				assert.Equal(t, string(gw.Spec.Listeners[1].Name), string(sp[1].Name), "sp 2 - name")
				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto-tcp")
				assert.Equal(t, string(gw.Spec.Listeners[1].Port), string(sp[1].Port), "sp 2 - port")
			},
		},
		{
			name: "lb service - multi-listener-multi-proto - dummy mixed proto annotation",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGw.DeepCopy()
				mixedProtoAnnotation := map[string]string{
					opdefault.MixedProtocolAnnotationKey: "dummy",
				}
				w.ObjectMeta.SetAnnotations(mixedProtoAnnotation)
				c.gws = []gwapiv1.Gateway{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 2, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")
				emp, found := as[opdefault.MixedProtocolAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.NotEqual(t, opdefault.MixedProtocolAnnotationValue, emp, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - multi-listener-multi-proto - mixed-proto enabled in GwConfig",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.MixedProtocolAnnotationKey] =
					opdefault.MixedProtocolAnnotationValue
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 2, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")
				emp, found := as[opdefault.MixedProtocolAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, opdefault.MixedProtocolAnnotationValue, emp, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto-udp")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
				assert.Equal(t, string(gw.Spec.Listeners[1].Name), string(sp[1].Name), "sp 2 - name")
				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto-tcp")
				assert.Equal(t, string(gw.Spec.Listeners[1].Port), string(sp[1].Port), "sp 2 - port")
			},
		},
		{
			name: "lb service - multi-listener-multi-proto - mixed-proto enabled in GwConfig but disabled in Gateway",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.MixedProtocolAnnotationKey] = opdefault.MixedProtocolAnnotationValue
				c.cfs = []stnrgwv1.GatewayConfig{*w}
				gw := testutils.TestGw.DeepCopy()
				mixedProtoAnnotation := map[string]string{
					opdefault.MixedProtocolAnnotationKey: "false",
				}
				gw.ObjectMeta.SetAnnotations(mixedProtoAnnotation)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 2, "annotations len")
				gwa, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, store.GetObjectKey(gw), gwa, "annotation ok")
				emp, found := as[opdefault.MixedProtocolAnnotationKey]
				assert.True(t, found, "annotation found")
				assert.Equal(t, "false", emp, "annotation ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto-udp")
			},
		},
		{
			name: "lb service - lb annotations",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["test"] = "testval"
				w.Spec.LoadBalancerServiceAnnotations["dummy"] = "dummyval"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayKey]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetName(), lab, "label ok")
				lab, found = ls[opdefault.RelatedGatewayNamespace]
				assert.True(t, found, "label found")
				assert.Equal(t, gw.GetNamespace(), lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 4, "annotations len")

				a, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["test"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "testval", a, "annotation 2 ok")

				a, found = as["dummy"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "dummyval", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb labels",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceLabels = map[string]string{
					"test":  "testval",
					"dummy": "dummyval",
					// mandatory labels must win
					opdefault.OwnedByLabelKey: "someone-else",
					// invalid labels are ignored
					"invalid key":   "val",
					"invalid-value": "has spaces",
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")

				ls := s.GetLabels()
				assert.Len(t, ls, 5, "labels len")
				assert.Equal(t, opdefault.OwnedByLabelValue, ls[opdefault.OwnedByLabelKey],
					"mandatory label wins")
				assert.Equal(t, gw.GetName(), ls[opdefault.RelatedGatewayKey], "label ok")
				assert.Equal(t, gw.GetNamespace(), ls[opdefault.RelatedGatewayNamespace], "label ok")
				assert.Equal(t, "testval", ls["test"], "user label 1 ok")
				assert.Equal(t, "dummyval", ls["dummy"], "user label 2 ok")
				assert.NotContains(t, ls, "invalid key", "invalid key ignored")
				assert.NotContains(t, ls, "invalid-value", "invalid value ignored")

				as := s.GetAnnotations()
				a, found := as[opdefault.ManagedLabelsAnnotationKey]
				assert.True(t, found, "managed-labels annotation found")
				assert.Equal(t, "dummy,test", a, "managed-labels annotation ok")

				// selector is unaffected
				assert.NotContains(t, s.Spec.Selector, "test", "selector")
			},
		},
		{
			name: "lb service - lb labels pruned on update",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceLabels = map[string]string{"keep": "newval"}
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				// existing LB service from a previous reconcile
				s := testutils.TestSvc.DeepCopy()
				s.SetName(testutils.TestGw.GetName())
				s.SetLabels(map[string]string{
					opdefault.OwnedByLabelKey:         opdefault.OwnedByLabelValue,
					opdefault.RelatedGatewayNamespace: testutils.TestGw.GetNamespace(),
					opdefault.RelatedGatewayKey:       testutils.TestGw.GetName(),
					"keep":                            "oldval",
					"stale":                           "val",
					"external":                        "val",
				})
				s.SetAnnotations(map[string]string{
					opdefault.RelatedGatewayKey:          store.GetObjectKey(&testutils.TestGw),
					opdefault.ManagedLabelsAnnotationKey: "keep,stale",
				})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc update")

				ls := s.GetLabels()
				assert.Len(t, ls, 5, "labels len")
				assert.Equal(t, "newval", ls["keep"], "managed label updated")
				assert.NotContains(t, ls, "stale", "removed managed label pruned")
				assert.Equal(t, "val", ls["external"], "external label preserved")

				assert.Equal(t, "keep", s.GetAnnotations()[opdefault.ManagedLabelsAnnotationKey],
					"managed-labels annotation updated")
			},
		},
		{
			name: "lb service - all lb labels removed",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.SetName(testutils.TestGw.GetName())
				s.SetLabels(map[string]string{"stale": "val", "external": "val"})
				s.SetAnnotations(map[string]string{
					opdefault.RelatedGatewayKey:          store.GetObjectKey(&testutils.TestGw),
					opdefault.ManagedLabelsAnnotationKey: "stale",
				})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc update")

				ls := s.GetLabels()
				assert.NotContains(t, ls, "stale", "removed managed label pruned")
				assert.Equal(t, "val", ls["external"], "external label preserved")

				a, found := s.GetAnnotations()[opdefault.ManagedLabelsAnnotationKey]
				assert.True(t, found, "managed-labels annotation kept for the updater")
				assert.Equal(t, "", a, "managed-labels annotation empty")
			},
		},
		{
			name: "lb service - empty managed-labels annotation dropped",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// the reconcile after all labels were removed: the annotation is empty
				s := testutils.TestSvc.DeepCopy()
				s.SetName(testutils.TestGw.GetName())
				s.SetLabels(map[string]string{"external": "val"})
				s.SetAnnotations(map[string]string{
					opdefault.RelatedGatewayKey:          store.GetObjectKey(&testutils.TestGw),
					opdefault.ManagedLabelsAnnotationKey: "",
				})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc update")

				assert.Equal(t, "val", s.GetLabels()["external"], "external label preserved")
				assert.NotContains(t, s.GetAnnotations(), opdefault.ManagedLabelsAnnotationKey,
					"empty managed-labels annotation dropped")
			},
		},
		{
			name: "lb service - invalid lb annotations ignored",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = map[string]string{
					"valid": "val",
					// case does not matter for annotation keys
					"Example.COM/Key": "val",
					// invalid annotations are ignored
					"invalid key": "val",
					"a/b/c":       "val",
					// STUNner-specific annotations are validated too
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")

				as := s.GetAnnotations()
				assert.Len(t, as, 5, "annotations len")
				assert.Equal(t, "val", as["valid"], "valid annotation ok")
				assert.Equal(t, "val", as["Example.COM/Key"], "mixed-case annotation ok")
				assert.NotContains(t, as, "invalid key", "invalid key ignored")
				assert.NotContains(t, as, "a/b/c", "invalid prefix ignored")
				assert.Equal(t, "NodePort", as[opdefault.ServiceTypeAnnotationKey], "stunner annotation ok")
				assert.Equal(t, "Example.COM/Key,valid", as[opdefault.ManagedAnnotationsAnnotationKey],
					"invalid annotations not tracked")
				assert.Equal(t, corev1.ServiceTypeNodePort, s.Spec.Type, "service type")
			},
		},
		{
			name: "lb service - lb annotations pruned on update",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = map[string]string{
					"keep": "newval",
					// STUNner-specific annotations are not tracked
					opdefault.ServiceTypeAnnotationKey: "NodePort",
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{"from-gw": "val"})
				c.gws = []gwapiv1.Gateway{*gw}

				// existing LB service from a previous reconcile
				s := testutils.TestSvc.DeepCopy()
				s.SetName(testutils.TestGw.GetName())
				s.SetAnnotations(map[string]string{
					opdefault.RelatedGatewayKey:               store.GetObjectKey(&testutils.TestGw),
					opdefault.ManagedAnnotationsAnnotationKey: "keep,stale,stale-gw",
					"keep":     "oldval",
					"stale":    "val",
					"stale-gw": "val",
					// e.g., set by the cloud provider
					"external": "val",
				})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc update")

				as := s.GetAnnotations()
				assert.Len(t, as, 6, "annotations len")
				assert.Equal(t, "newval", as["keep"], "managed annotation updated")
				assert.Equal(t, "val", as["from-gw"], "gw annotation added")
				assert.NotContains(t, as, "stale", "removed gw-config annotation pruned")
				assert.NotContains(t, as, "stale-gw", "removed gw annotation pruned")
				assert.Equal(t, "val", as["external"], "external annotation preserved")
				assert.Equal(t, "NodePort", as[opdefault.ServiceTypeAnnotationKey],
					"stunner annotation kept")
				assert.Equal(t, "from-gw,keep", as[opdefault.ManagedAnnotationsAnnotationKey],
					"managed-annotations annotation updated")
				assert.Equal(t, corev1.ServiceTypeNodePort, s.Spec.Type, "service type")
			},
		},
		{
			name: "lb service - all lb annotations removed",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.SetName(testutils.TestGw.GetName())
				s.SetAnnotations(map[string]string{
					opdefault.RelatedGatewayKey:               store.GetObjectKey(&testutils.TestGw),
					opdefault.ManagedAnnotationsAnnotationKey: "stale",
					"stale":    "val",
					"external": "val",
				})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc update")

				as := s.GetAnnotations()
				assert.NotContains(t, as, "stale", "removed managed annotation pruned")
				assert.Equal(t, "val", as["external"], "external annotation preserved")

				a, found := as[opdefault.ManagedAnnotationsAnnotationKey]
				assert.True(t, found, "managed-annotations annotation kept for the updater")
				assert.Equal(t, "", a, "managed-annotations annotation empty")
			},
		},
		{
			name: "lb service - untracked lb annotations not pruned",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// a Service created before the operator started tracking annotations
				s := testutils.TestSvc.DeepCopy()
				s.SetName(testutils.TestGw.GetName())
				s.SetAnnotations(map[string]string{
					opdefault.RelatedGatewayKey: store.GetObjectKey(&testutils.TestGw),
					"legacy":                    "val",
				})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc update")

				as := s.GetAnnotations()
				assert.Equal(t, "val", as["legacy"], "untracked annotation preserved")
				assert.NotContains(t, as, opdefault.ManagedAnnotationsAnnotationKey,
					"nothing to track")
			},
		},
		{
			name: "lb service - lb health check annotations (aws)",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 4, "annotations len")

				a, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/aws-load-balancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "8080", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "HTTP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")

				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, int32(8080), sp[1].Port, "sp 2 - port")
			},
		},
		{
			name: "lb service - lb health check disabled (aws)",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/aws-load-balancer-healthcheck-protocol"] = "HTTP"
				w.Spec.LoadBalancerServiceAnnotations[opdefault.DisableHealthCheckExposeAnnotationKey] =
					opdefault.DisableHealthCheckExposeAnnotationValue
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				sp := s.Spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb health check annotations (do)",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 4, "annotations len")

				a, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "8080", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "HTTP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")

				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, int32(8080), sp[1].Port, "sp 2 - port")
			},
		},
		{
			name: "lb service - lb health check annotations not an int port (do)",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "eighty"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 4, "annotations len")

				a, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "eighty", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "HTTP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb health check annotations UDP protocol (do)",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "UDP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				ls := s.GetLabels()
				assert.Len(t, ls, 3, "labels len")
				lab, found := ls[opdefault.OwnedByLabelKey]
				assert.True(t, found, "label found")
				assert.Equal(t, opdefault.OwnedByLabelValue, lab, "label ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 4, "annotations len")

				a, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "8080", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "UDP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb annotations from gwConf override from Gw",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["test"] = "testval"
				w.Spec.LoadBalancerServiceAnnotations["dummy"] = "dummyval"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann["test"] = "testval"         // same
				ann["dummy"] = "something-else" // overrride
				ann["extra"] = "extraval"       // extra
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 5, "annotations len")

				a, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["test"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "testval", a, "annotation 2 ok")

				a, found = as["dummy"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "something-else", a, "annotation 3 ok")

				a, found = as["extra"]
				assert.True(t, found, "annotation 4 found")
				assert.Equal(t, "extraval", a, "annotation 4 ok")

				a, found = as[opdefault.ManagedAnnotationsAnnotationKey]
				assert.True(t, found, "managed-annotations annotation found")
				assert.Equal(t, "dummy,extra,test", a, "gw-config and gw annotations tracked")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 1, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")
			},
		},
		{
			name: "lb service - lb annotations health check from gwConf override from Gw",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "UDP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP" // overrride
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				as := s.GetAnnotations()
				assert.Len(t, as, 4, "annotations len")

				a, found := as[opdefault.RelatedGatewayKey]
				assert.True(t, found, "annotation 1 found")
				assert.Equal(t, store.GetObjectKey(gw), a, "annotation 1 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"]
				assert.True(t, found, "annotation 2 found")
				assert.Equal(t, "8080", a, "annotation 2 ok")

				a, found = as["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"]
				assert.True(t, found, "annotation 3 found")
				assert.Equal(t, "HTTP", a, "annotation 3 ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				sp := spec.Ports
				assert.Len(t, sp, 2, "service-port len")
				assert.Equal(t, string(gw.Spec.Listeners[0].Name), string(sp[0].Name), "sp 1 - name")
				assert.Equal(t, "UDP", string(sp[0].Protocol), "sp 1 - proto")
				assert.Equal(t, string(gw.Spec.Listeners[0].Port), string(sp[0].Port), "sp 1 - port")

				assert.Equal(t, "TCP", string(sp[1].Protocol), "sp 2 - proto")
				assert.Equal(t, int32(8080), sp[1].Port, "sp 2 - port")
			},
		},
		{
			name: "lb service - default svc type",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
			},
		},
		{
			name: "lb service - svc type ClusterIP from gwConf",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeClusterIP, spec.Type, "svc type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				// clusterIP services do not need a health-check service-port
				found := false
				for _, sp := range spec.Ports {
					if sp.Protocol == "TCP" && sp.Port == int32(8080) {
						found = true
					}
				}
				assert.False(t, found, "health-check port exists")
			},
		},
		{
			name: "lb service - svc type NodePort from gwConf",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "NodePort"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeNodePort, spec.Type, "svc type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				// NodePort services do not need a health-check service-port
				found := false
				for _, sp := range spec.Ports {
					if sp.Protocol == "TCP" && sp.Port == int32(8080) {
						found = true
					}
				}
				assert.False(t, found, "health-check port exists")
			},
		},
		{
			name: "lb service - svc type from svc annotation",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
				ann["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				ann["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeClusterIP, spec.Type, "svc type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				// ClusterIPservices do not need a health-check service-port
				found := false
				for _, sp := range spec.Ports {
					if sp.Protocol == "TCP" && sp.Port == int32(8080) {
						found = true
					}
				}
				assert.False(t, found, "health-check port exists")
			},
		},
		{
			name: "lb service - nodeport svc override gwConf from svc annotation",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] = "ClusterIP"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				w.Spec.LoadBalancerServiceAnnotations["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann[opdefault.ServiceTypeAnnotationKey] = "NodePort"
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeNodePort, spec.Type, "svc type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				// NodePort services do not need a health-check service-port
				found := false
				for _, sp := range spec.Ports {
					if sp.Protocol == "TCP" && sp.Port == int32(8080) {
						found = true
					}
				}
				assert.False(t, found, "health-check port exists")
			},
		},
		{
			name: "lb service - svc type NodePort from gw annotation",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				ann := make(map[string]string)
				ann[opdefault.ServiceTypeAnnotationKey] = "NodePort"
				ann["service.beta.kubernetes.io/do-loadbalancer-healthcheck-port"] = "8080"
				ann["service.beta.kubernetes.io/do-loadbalancer-healthcheck-protocol"] = "HTTP"
				gw.SetAnnotations(ann)
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeNodePort, spec.Type, "svc type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")

				// NodePort services do not need a health-check service-port
				found := false
				for _, sp := range spec.Ports {
					if sp.Protocol == "TCP" && sp.Port == int32(8080) {
						found = true
					}
				}
				assert.False(t, found, "health-check port exists")
			},
		},
		{
			name: "lb service - disabling session affinity (Oracle Kubernetes)",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.DisableSessionAffinityAnnotationKey] =
					opdefault.DisableSessionAffinityAnnotationValue
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, gw.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, gw.GetName(), s.GetName(), "name ok")

				assert.True(t, string(s.Spec.SessionAffinity) == "" ||
					s.Spec.SessionAffinity == corev1.ServiceAffinityNone, "session affinity disabled")
			},
		},
		{
			name: "public address hint in Gateway Spec",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				at := gwapiv1.IPAddressType
				gw.Spec.Addresses = []gwapiv1.GatewaySpecAddress{
					{
						Type:  &at,
						Value: "1.1.1.1",
					},
					{
						Type:  &at,
						Value: "1.2.3.4",
					},
				}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
				assert.Equal(t, s.Spec.LoadBalancerIP, "1.1.1.1", "svc loadbalancerip")
			},
		},
		{
			name: "conflicting listeners share no Service port",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{
					{Name: "udp-1", Port: 3478, Protocol: "TURN-UDP"},
					{Name: "udp-2", Port: 3478, Protocol: "TURN-UDP"},
					{Name: "dtls", Port: 3478, Protocol: "TURN-DTLS"},
				}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")

				s, _ := r.createLbService4Gateway(c, gws[0])
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, []corev1.ServicePort{{Name: "udp-1", Protocol: corev1.ProtocolUDP, Port: 3478}},
					s.Spec.Ports, "the first listener gets the port")
			},
		},
		{
			name: "lb service - ext traffic policy set gwConf ",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ExternalTrafficPolicyAnnotationKey] =
					opdefault.ExternalTrafficPolicyAnnotationValue
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, corev1.ServiceExternalTrafficPolicyLocal,
					spec.ExternalTrafficPolicy, "ext traffic policy local")
			},
		},
		{
			name: "lb service - ext traffic policy set in gw ",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ExternalTrafficPolicyAnnotationKey] = "dummy"
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				// override
				gw := testutils.TestGw.DeepCopy()
				as := make(map[string]string)
				as[opdefault.ExternalTrafficPolicyAnnotationKey] =
					opdefault.ExternalTrafficPolicyAnnotationValue
				gw.SetAnnotations(as)
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")

				assert.Equal(t, corev1.ServiceExternalTrafficPolicyLocal,
					spec.ExternalTrafficPolicy, "ext traffic policy local")
			},
		},
		{
			name: "lb service - ext traffic policy must not be set for ClusterIP svc",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ServiceTypeAnnotationKey] =
					"ClusterIP"
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ExternalTrafficPolicyAnnotationKey] =
					opdefault.ExternalTrafficPolicyAnnotationValue
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				as := make(map[string]string)
				as[opdefault.ExternalTrafficPolicyAnnotationKey] = "dummy"
				gw.SetAnnotations(as)
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeClusterIP, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")
			},
		},
		{
			name: "lb service - ext traffic policy invalid",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = make(map[string]string)
				w.Spec.LoadBalancerServiceAnnotations[opdefault.ExternalTrafficPolicyAnnotationKey] =
					opdefault.ExternalTrafficPolicyAnnotationValue
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				as := make(map[string]string)
				as[opdefault.ExternalTrafficPolicyAnnotationKey] = "dummy"
				gw.SetAnnotations(as)
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("gateway-1-listener-udp"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				spec := s.Spec
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type, "lb type")
				assert.Equal(t, defaultExternalTrafficPolicy, spec.ExternalTrafficPolicy,
					"ext traffic policy default")
			},
		},
		{
			name: "lb service - JSON formatted nodeport annotation in the GatewayConfig enforced",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = map[string]string{
					opdefault.NodePortAnnotationKey: "{\"force-np1\":101,\"force-np2\":102,\"force-np3\":103}",
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("random-np"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np1"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np2"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
				ports := s.Spec.Ports
				assert.Len(t, ports, 3, "service-port len")
				assert.Equal(t, "random-np", ports[0].Name, "port 1 name")
				assert.Equal(t, int32(0), ports[0].NodePort, "port 1 np") // default
				assert.Equal(t, "force-np1", ports[1].Name, "port 1 name")
				assert.Equal(t, int32(101), ports[1].NodePort, "port 1 np")
				assert.Equal(t, "force-np2", ports[2].Name, "port 2 name")
				assert.Equal(t, int32(102), ports[2].NodePort, "port 2 np")
			},
		},
		{
			name: "lb service - JSON formatted nodeport annotation enforced",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = map[string]string{
					opdefault.NodePortAnnotationKey: `{\"dummy-data-in-the-wrong-format`,
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{
					opdefault.NodePortAnnotationKey: "{\"force-np1\":101,\"force-np2\":102,\"force-np3\":103}",
				})
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("random-np"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np1"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np2"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
				ports := s.Spec.Ports
				assert.Len(t, ports, 3, "service-port len")
				assert.Equal(t, "random-np", ports[0].Name, "port 1 name")
				assert.Equal(t, int32(0), ports[0].NodePort, "port 1 np") // default
				assert.Equal(t, "force-np1", ports[1].Name, "port 1 name")
				assert.Equal(t, int32(101), ports[1].NodePort, "port 1 np")
				assert.Equal(t, "force-np2", ports[2].Name, "port 2 name")
				assert.Equal(t, int32(102), ports[2].NodePort, "port 2 np")
			},
		},
		{
			name: "lb service - JSON formatted targetport annotation in the GatewayConfig enforced",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = map[string]string{
					opdefault.TargetPortAnnotationKey: "{\"force-np1\":101,\"force-np2\":102,\"force-np3\":103}",
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("random-np"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np1"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np2"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
				ports := s.Spec.Ports
				assert.Len(t, ports, 3, "service-port len")
				assert.Equal(t, "random-np", ports[0].Name, "port 1 name")
				assert.Equal(t, intstr.FromInt(0), ports[0].TargetPort, "port 1 np") // default
				assert.Equal(t, "force-np1", ports[1].Name, "port 1 name")
				assert.Equal(t, intstr.FromInt(101), ports[1].TargetPort, "port 1 np")
				assert.Equal(t, "force-np2", ports[2].Name, "port 2 name")
				assert.Equal(t, intstr.FromInt(102), ports[2].TargetPort, "port 2 np")

				// we must have received a targetport map
				assert.NotNil(t, tp, "targetports")
				assert.Len(t, tp, 3, "target-port len")
				p, ok := tp["force-np1"]
				assert.True(t, ok, "targetport 1 exists")
				assert.Equal(t, 101, p, "targetport 1")
				p, ok = tp["force-np2"]
				assert.True(t, ok, "targetport 2 exists")
				assert.Equal(t, 102, p, "targetport 2")
				p, ok = tp["force-np3"]
				assert.True(t, ok, "targetport 3 exists")
				assert.Equal(t, 103, p, "targetport 3")
			},
		},
		{
			name: "lb service - JSON formatted targetport annotation enforced",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.LoadBalancerServiceAnnotations = map[string]string{
					opdefault.TargetPortAnnotationKey: `{\"dummy-data-in-the-wrong-format`,
				}
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{
					opdefault.TargetPortAnnotationKey: "{\"force-np1\":101,\"force-np2\":102,\"force-np3\":103}",
				})
				gw.Spec.Listeners = []gwapiv1.Listener{{
					Name:     gwapiv1.SectionName("random-np"),
					Port:     gwapiv1.PortNumber(1),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np1"),
					Port:     gwapiv1.PortNumber(2),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}, {
					Name:     gwapiv1.SectionName("force-np2"),
					Port:     gwapiv1.PortNumber(3),
					Protocol: gwapiv1.ProtocolType("UDP"),
				}}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
				ports := s.Spec.Ports
				assert.Len(t, ports, 3, "service-port len")
				assert.Equal(t, "random-np", ports[0].Name, "port 1 name")
				assert.Equal(t, intstr.FromInt(0), ports[0].TargetPort, "port 1 np") // default
				assert.Equal(t, "force-np1", ports[1].Name, "port 1 name")
				assert.Equal(t, intstr.FromInt(101), ports[1].TargetPort, "port 1 np")
				assert.Equal(t, "force-np2", ports[2].Name, "port 2 name")
				assert.Equal(t, intstr.FromInt(102), ports[2].TargetPort, "port 2 np")

				// we must have received a targetport map
				assert.NotNil(t, tp, "targetports")
				assert.Len(t, tp, 3, "target-port len")
				p, ok := tp["force-np1"]
				assert.True(t, ok, "targetport 1 exists")
				assert.Equal(t, 101, p, "targetport 1")
				p, ok = tp["force-np2"]
				assert.True(t, ok, "targetport 2 exists")
				assert.Equal(t, 102, p, "targetport 2")
				p, ok = tp["force-np3"]
				assert.True(t, ok, "targetport 3 exists")
				assert.Equal(t, 103, p, "targetport 3")
			},
		},
		{
			name: "lb service - service nodeport retained, targetport follows the listener",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				// make sure UDP and TCP are both handled
				gw := testutils.TestGw.DeepCopy()
				mixedProtoAnnotation := map[string]string{
					opdefault.MixedProtocolAnnotationKey: "true",
				}
				gw.ObjectMeta.SetAnnotations(mixedProtoAnnotation)
				c.gws = []gwapiv1.Gateway{*gw}

				s1 := testutils.TestSvc.DeepCopy()
				s1.SetName("gateway-1")
				s1.SetNamespace("testnamespace")
				s1.Spec.Ports = []corev1.ServicePort{
					{
						Name:       "gateway-1-listener-udp",
						Protocol:   corev1.ProtocolUDP,
						Port:       1,
						NodePort:   30001,
						TargetPort: intstr.FromInt(7001), // defaulted to an old listener port
					},
					{
						Name:       "dummy",
						Protocol:   corev1.ProtocolTCP,
						Port:       123,
						NodePort:   31234,
						TargetPort: intstr.FromString("dummy"),
					},
				}
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, _ := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")
				assert.Equal(t, corev1.ServiceTypeLoadBalancer, s.Spec.Type, "lb type")
				ports := s.Spec.Ports
				assert.Len(t, ports, 2, "service-port len")
				assert.Equal(t, "gateway-1-listener-udp", ports[0].Name, "port 1 name")
				assert.Equal(t, int32(30001), ports[0].NodePort, "port 1 np") // default
				assert.Equal(t, intstr.IntOrString{}, ports[0].TargetPort, "port 1 tp")
				assert.Equal(t, "gateway-1-listener-tcp", ports[1].Name, "port 1 name")
				assert.Equal(t, int32(0), ports[1].NodePort, "port 2 np")
				assert.Equal(t, intstr.IntOrString{}, ports[1].TargetPort, "port 2 tp")
			},
		},
		{
			name: "lb service - STUNner-specific annotation removed unless GW also sets it",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				as := make(map[string]string)
				// valid in both
				as[opdefault.ExternalTrafficPolicyAnnotationKey] = "testpolicy"
				// valid in both but gw overrides
				as[opdefault.ManagedDataplaneDisabledAnnotationKey] = "dummymanageddisabled"
				// only in gw
				as[opdefault.MixedProtocolAnnotationKey] = "testmixedproto"
				gw.SetAnnotations(as)
				c.gws = []gwapiv1.Gateway{*gw}

				s1 := testutils.TestSvc.DeepCopy()
				as = make(map[string]string)
				// valid in both
				as[opdefault.ExternalTrafficPolicyAnnotationKey] = "testpolicy"
				// valid in both but gw overrides
				as[opdefault.ManagedDataplaneDisabledAnnotationKey] = "random"
				// only in svc
				as[opdefault.NodePortAnnotationKey] = "testnodeport"
				s1.SetAnnotations(as)
				s1.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s1}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")
				gw := gws[0]

				s, tp := r.createLbService4Gateway(c, gw)
				assert.NotNil(t, s, "svc create")
				assert.Nil(t, tp, "targetports")
				assert.Equal(t, c.gwConf.GetNamespace(), s.GetNamespace(), "namespace ok")

				as := s.GetAnnotations()
				// valid in both
				v, ok := as[opdefault.ExternalTrafficPolicyAnnotationKey]
				assert.True(t, ok, "ann valid in both - ok")
				assert.Equal(t, "testpolicy", v, "ann valid in both - val ok")
				// valid in both but gw overrides
				v, ok = as[opdefault.ManagedDataplaneDisabledAnnotationKey]
				assert.True(t, ok, "ann valid in both - ok")
				assert.Equal(t, "dummymanageddisabled", v, "ann valid in both - val ok")
				// only in gw
				v, ok = as[opdefault.MixedProtocolAnnotationKey]
				assert.True(t, ok, "ann valid in both - ok")
				assert.Equal(t, "testmixedproto", v, "ann valid in both - val ok")
				// only in svc
				_, ok = as[opdefault.NodePortAnnotationKey]
				assert.False(t, ok, "ann valid in both - ok")
			},
		},
	})
}

func TestRenderNodePortPublicAddr(t *testing.T) {
	noExternal := testutils.TestNode.DeepCopy()
	noExternal.Status.Addresses = noExternal.Status.Addresses[:len(noExternal.Status.Addresses)-1]
	second := testutils.TestNode.DeepCopy()
	second.SetName("node-2")
	noAddress := testutils.TestNode.DeepCopy()
	noAddress.Status.Addresses = []corev1.NodeAddress{}

	for _, tc := range []struct {
		name  string
		nodes []corev1.Node
		addr  string
	}{
		{name: "the first node with a valid address", nodes: []corev1.Node{*noExternal, *second}, addr: "1.2.3.4"},
		{name: "no node address", nodes: []corev1.Node{*noAddress}},
	} {
		renderTester(t, []renderTestConfig{{
			name:  tc.name,
			cls:   []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:   []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:   []gwapiv1.Gateway{testutils.TestGw},
			rs:    []stnrgwv1.UDPRoute{},
			nodes: tc.nodes,
			svcs:  []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				s := testutils.TestSvc.DeepCopy()
				s.Spec.Type = corev1.ServiceTypeNodePort
				s.Spec.Ports[0].NodePort = 30001
				s.Spec.Ports[1].NodePort = 30002
				s.SetOwnerReferences([]metav1.OwnerReference{{
					APIVersion: gwapiv1.GroupVersion.String(),
					Kind:       "Gateway",
					UID:        testutils.TestGw.GetUID(),
					Name:       testutils.TestGw.GetName(),
				}})
				c.svcs = []corev1.Service{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")

				addrs, err := r.getPublicAddr(gws[0])
				if tc.addr == "" {
					assert.True(t, IsNonCriticalError(err, PublicListenerAddressNotFound), "no public address")
					assert.Empty(t, addrs[0].addr, "no public address")
					return
				}
				assert.NoError(t, err, "public address")
				assert.Len(t, addrs, 2, "public addr-port len")
				assert.Equal(t, tc.addr, addrs[0].addr, "the node address")
				assert.Equal(t, 30001, addrs[0].port, "the node port")
			},
		}})
	}
}

func TestRenderServiceNodePortAnnotation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		annotation string
		nodePorts  []int32
	}{
		{name: "JSON", annotation: `{"force-np1":101,"force-np2":102}`, nodePorts: []int32{0, 101, 102}},
		{name: "JSON without curlies", annotation: `"force-np1":101,"force-np2":102`, nodePorts: []int32{0, 101, 102}},
		{name: "empty object", annotation: `{}`, nodePorts: []int32{0, 0, 0}},
		{name: "empty string", annotation: ``, nodePorts: []int32{0, 0, 0}},
		{name: "unterminated list ignored", annotation: `["force-np1":101`, nodePorts: []int32{0, 0, 0}},
		{name: "string ignored", annotation: `"dummy"`, nodePorts: []int32{0, 0, 0}},
		{name: "nested object ignored", annotation: `{"force-np1":101,"c":{"a":1}}`, nodePorts: []int32{0, 0, 0}},
	} {
		renderTester(t, []renderTestConfig{{
			name: "nodeport annotation: " + tc.name,
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{testutils.TestGw},
			rs:   []stnrgwv1.UDPRoute{},
			svcs: []corev1.Service{testutils.TestSvc},
			prep: func(c *renderTestConfig) {
				gw := testutils.TestGw.DeepCopy()
				gw.SetAnnotations(map[string]string{opdefault.NodePortAnnotationKey: tc.annotation})
				gw.Spec.Listeners = []gwapiv1.Listener{
					{Name: "random-np", Port: 1, Protocol: "UDP"},
					{Name: "force-np1", Port: 2, Protocol: "UDP"},
					{Name: "force-np2", Port: 3, Protocol: "UDP"},
				}
				c.gws = []gwapiv1.Gateway{*gw}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")
				gws := r.getGateways4Class(c)
				assert.Len(t, gws, 1, "gateways for class")

				s, _ := r.createLbService4Gateway(c, gws[0])
				assert.NotNil(t, s, "svc create")
				nps := []int32{}
				for _, p := range s.Spec.Ports {
					nps = append(nps, p.NodePort)
				}
				assert.Equal(t, tc.nodePorts, nps, "node ports")
			},
		}})
	}
}

func TestRenderServiceStunnerAnnotations(t *testing.T) {
	renderTester(t, []renderTestConfig{{
		name: "stale STUNner annotations pruned, lookalikes kept",
		cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
		cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
		gws:  []gwapiv1.Gateway{testutils.TestGw},
		rs:   []stnrgwv1.UDPRoute{},
		svcs: []corev1.Service{testutils.TestSvc},
		prep: func(c *renderTestConfig) {
			// The Service of the Gateway exists already.
			s := testutils.TestSvc.DeepCopy()
			s.SetName(testutils.TestGw.GetName())
			s.SetNamespace(testutils.TestGw.GetNamespace())
			s.SetAnnotations(map[string]string{
				"Stunner.L7mp.io/Service-Type":    "NodePort",
				"stunner.l7mp.io.example.com/foo": "bar",
				"stunner.l7mp.io-foo/bar":         "baz",
				"example.com/foo":                 "qux",
			})
			c.svcs = []corev1.Service{*s}
		},
		tester: func(t *testing.T, r *renderer) {
			gc, err := r.getGatewayClass()
			assert.NoError(t, err, "gw-class found")
			c := &RenderContext{gc: gc, log: log}
			c.gwConf, err = r.getGatewayConfig4Class(c)
			assert.NoError(t, err, "gw-conf found")
			gws := r.getGateways4Class(c)
			assert.Len(t, gws, 1, "gateways for class")

			s, _ := r.createLbService4Gateway(c, gws[0])
			assert.NotNil(t, s, "svc create")
			as := s.GetAnnotations()
			assert.NotContains(t, as, "Stunner.L7mp.io/Service-Type",
				"a STUNner annotation the Gateway no longer has is pruned, whatever the case")
			assert.Equal(t, "bar", as["stunner.l7mp.io.example.com/foo"], "lookalike domain kept")
			assert.Equal(t, "baz", as["stunner.l7mp.io-foo/bar"], "lookalike prefix kept")
			assert.Equal(t, "qux", as["example.com/foo"], "other annotation kept")
		},
	}})
}

func TestRenderServiceExternalNameType(t *testing.T) {
	renderTester(t, []renderTestConfig{{
		name: "an ExternalName service type falls back to the default",
		cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
		cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
		gws:  []gwapiv1.Gateway{testutils.TestGw},
		rs:   []stnrgwv1.UDPRoute{},
		svcs: []corev1.Service{testutils.TestSvc},
		prep: func(c *renderTestConfig) {
			gw := testutils.TestGw.DeepCopy()
			gw.SetAnnotations(map[string]string{opdefault.ServiceTypeAnnotationKey: "ExternalName"})
			c.gws = []gwapiv1.Gateway{*gw}
		},
		tester: func(t *testing.T, r *renderer) {
			gc, err := r.getGatewayClass()
			assert.NoError(t, err, "gw-class found")
			c := &RenderContext{gc: gc, log: log}
			c.gwConf, err = r.getGatewayConfig4Class(c)
			assert.NoError(t, err, "gw-conf found")

			gws := r.getGateways4Class(c)
			assert.Len(t, gws, 1, "gateways for class")

			s, _ := r.createLbService4Gateway(c, gws[0])
			assert.NotNil(t, s, "svc create")
			assert.Equal(t, opdefault.DefaultServiceType, s.Spec.Type, "default service type")
		},
	}})
}
