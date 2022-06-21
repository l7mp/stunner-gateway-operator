package renderer

import (
	// "context"
	"encoding/json"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	// "sigs.k8s.io/controller-runtime/pkg/log/zap"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/l7mp/stunner-gateway-operator/internal/config"
	"github.com/l7mp/stunner-gateway-operator/internal/event"
	// "github.com/l7mp/stunner-gateway-operator/internal/store"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"
)

func TestRenderE2E(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "piecewise render",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{},
			rs:   []gatewayv1alpha2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				assert.Equal(t, "gatewayclass-ok", gc.GetName(),
					"gatewayclass name")

				gwConf, err := r.getGatewayConfig4Class(gc)
				assert.NoError(t, err, "gw-conf found")
				assert.Equal(t, "gatewayconfig-ok", gwConf.GetName(),
					"gatewayconfig name")

				admin, err := r.renderAdmin(gwConf)
				assert.NoError(t, err, "admin rendered")
				assert.Equal(t, "testloglevel", admin.LogLevel, "log level")
				assert.Equal(t, config.DefaultStunnerdInstanceName,
					admin.Name, "stunnerd name")

				auth, err := r.renderAuth(gwConf)
				assert.NoError(t, err, "auth rendered")
				assert.Equal(t, stunnerconfv1alpha1.AuthTypePlainText.String(),
					auth.Type, "auth type")
				assert.Equal(t, "testrealm", auth.Realm, "realm")
				assert.Equal(t, "testuser", auth.Credentials["username"], "username")
				assert.Equal(t, "testpass", auth.Credentials["password"], "password")
			},
		},
		{
			name: "E2E test OK",
			cls:  []gatewayv1alpha2.GatewayClass{testGwClass},
			cfs:  []stunnerv1alpha1.GatewayConfig{testGwConfig},
			gws:  []gatewayv1alpha2.Gateway{testGw},
			rs:   []gatewayv1alpha2.UDPRoute{testUDPRoute},
			svcs: []corev1.Service{testSvc},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				e := event.NewEventRender()
				assert.NotNil(t, e, "render event create")
				e.Origin = "tester"
				e.Reason = "unit-test"

				u := event.NewEventUpdate()
				assert.NotNil(t, u, "update event create")

				err := r.Render(e, u)
				assert.NoError(t, err, "render success")

				// configmap
				cms := u.ConfigMaps.Objects()
				assert.Len(t, cms, 1, "configmap ready")
				o := cms[0]

				// objectmeta
				assert.Equal(t, o.GetName(), testStunnerConfig,
					"configmap name")
				assert.Equal(t, o.GetNamespace(),
					"testnamespace", "configmap namespace")

				cm, ok := o.(*corev1.ConfigMap)
				assert.True(t, ok, "configmap cast")

				// typemeta: do we have to build the object with a schema???
				// assert.Equal(t, corev1.GroupName, cm.TypeMeta.Kind, "group")
				// fmt.Printf("%#v\n", cm.GetObjectKind().GroupVersionKind())
				// assert.Equal(t, "v1", cm.TypeMeta.APIVersion, "version")
				// assert.Equal(t, "ConfigMap", cm.TypeMeta.Kind, "kind")

				jsonConf, found := cm.Data[config.DefaultStunnerdConfigfileName]
				assert.True(t, found, "configmap data: stunnerd.conf found")

				// try to unmarschal
				conf := stunnerconfv1alpha1.StunnerConfig{}
				err = json.Unmarshal([]byte(jsonConf), &conf)
				assert.NoError(t, err, "json unmarschal")

				assert.Equal(t, config.DefaultStunnerdInstanceName,
					conf.Admin.Name, "name")
				assert.Equal(t, testLogLevel, conf.Admin.LogLevel,
					"loglevel")

				assert.Equal(t, testRealm, conf.Auth.Realm, "realm")
				assert.Equal(t, "plaintext", conf.Auth.Type, "auth-type")
				assert.Equal(t, testUsername, conf.Auth.Credentials["username"],
					"username")
				assert.Equal(t, testPassword, conf.Auth.Credentials["password"],
					"password")

				lc := conf.Listeners[0]
				assert.Equal(t, "gateway-1-listener-udp", lc.Name, "name")
				assert.Equal(t, "UDP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 1, "route num")
				assert.Equal(t, lc.Routes[0], "udproute-ok", "udp route")

				lc = conf.Listeners[1]
				assert.Equal(t, "gateway-1-listener-tcp", lc.Name, "name")
				assert.Equal(t, "TCP", lc.Protocol, "proto")
				assert.Equal(t, "1.2.3.4", lc.PublicAddr, "public-ip")
				assert.Equal(t, int(testMinPort), lc.MinRelayPort, "min-port")
				assert.Equal(t, int(testMaxPort), lc.MaxRelayPort, "max-port")
				assert.Len(t, lc.Routes, 0, "route num")

				rc := conf.Clusters[0]
				assert.Equal(t, "udproute-ok", rc.Name, "cluster name")
				assert.Equal(t, "STRICT_DNS", rc.Type, "cluster type")
				assert.Len(t, rc.Endpoints, 1, "endpoints len")
				assert.Equal(t, "testservice-ok.testnamespace.svc.cluster.local",
					rc.Endpoints[0], "backend-ref")

				// fmt.Printf("%#v\n", cm.(*corev1.ConfigMap))
			},
		},
	})
}
