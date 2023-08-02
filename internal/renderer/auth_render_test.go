package renderer

import (
	// "context"
	// "fmt"
	"github.com/stretchr/testify/assert"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/l7mp/stunner-gateway-operator/internal/testutils"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

func TestRenderAuthRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "default auth ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "plaintext", auth.Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, auth.Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, auth.Credentials["password"],
					"password")
			},
		},
		{
			name: "longterm auth ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.StunnerConfig = "dummy"
				*w.Spec.Realm = "dummy"
				*w.Spec.AuthType = "longterm"
				s := "dummy"
				w.Spec.SharedSecret = &s
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, "dummy", auth.Realm, "realm")
				assert.Equal(t, "longterm", auth.Type, "auth-type")
				assert.Equal(t, "dummy", auth.Credentials["secret"], "secret")

			},
		},
		{
			name: "wrong auth-type errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "dummy"
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "plaintext no-username errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "plaintext"
				w.Spec.Username = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "plaintext no-password errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "plaintext"
				w.Spec.Password = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "auth type alias: static - ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "static"
				u, p := "testuser", "testpasswd"
				w.Spec.Username = &u
				w.Spec.Password = &p
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, "plaintext", auth.Type, "auth-type")
				assert.Equal(t, "testuser", auth.Credentials["username"], "username")
				assert.Equal(t, "testpasswd", auth.Credentials["password"], "password")
			},
		},
		{
			name: "lonterm no-secret errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "longterm"
				w.Spec.SharedSecret = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "auth type alias: timewindowed - ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.StunnerConfig = "dummy"
				*w.Spec.Realm = "dummy"
				*w.Spec.AuthType = "timewindowed"
				s := "dummy"
				w.Spec.SharedSecret = &s
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, "dummy", auth.Realm, "realm")
				assert.Equal(t, "longterm", auth.Type, "auth-type")
				assert.Equal(t, "dummy", auth.Credentials["secret"], "secret")

			},
		},
		{
			name: "auth type alias: ephemeral - ok",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1a2.Gateway{},
			rs:   []gwapiv1a2.UDPRoute{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.StunnerConfig = "dummy"
				*w.Spec.Realm = "dummy"
				*w.Spec.AuthType = "timewindowed"
				s := "dummy"
				w.Spec.SharedSecret = &s
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, "dummy", auth.Realm, "realm")
				assert.Equal(t, "longterm", auth.Type, "auth-type")
				assert.Equal(t, "dummy", auth.Credentials["secret"], "secret")

			},
		},
		// external auth tests
		{
			name:   "default external auth ok",
			cls:    []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1b1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1b1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "plaintext", auth.Type, "auth-type")
				assert.Equal(t, "ext-testuser", auth.Credentials["username"],
					"username")
				assert.Equal(t, "ext-testpass", auth.Credentials["password"],
					"password")
			},
		},
		{
			name:   "longterm external auth ok",
			cls:    []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1b1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1b1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}

				s := testutils.TestAuthSecret.DeepCopy()
				s.Data["type"] = []byte("longterm")
				s.Data["secret"] = []byte("ext-secret")
				c.ascrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "longterm", auth.Type, "auth-type")
				assert.Equal(t, "ext-secret", auth.Credentials["secret"],
					"secret")
			},
		},
		{
			name:   "wrong secret group errs",
			cls:    []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				group := gwapiv1b1.Group("dummy-group")
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Group: &group,
					Name:  gwapiv1b1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "renderAuth")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name:   "wrong secret kind errs",
			cls:    []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				kind := gwapiv1b1.Kind("dummy-kind")
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Kind: &kind,
					Name: gwapiv1b1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "renderAuth")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "missing secret errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1b1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1b1.ObjectName("dummy-secret"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "missing secret")
			},
		},
		{
			name:   "missing namespace ok",
			cls:    []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Name: gwapiv1b1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "plaintext", auth.Type, "auth-type")
				assert.Equal(t, "ext-testuser", auth.Credentials["username"],
					"username")
				assert.Equal(t, "ext-testpass", auth.Credentials["password"],
					"password")
			},
		},
		{
			name:   "external auth overrides inline",
			cls:    []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Name: gwapiv1b1.ObjectName("testauthsecret-ok"),
				}
				atype := "longterm"
				sharedSecret := "testsecret"
				w.Spec.AuthType = &atype
				w.Spec.SharedSecret = &sharedSecret
				c.cfs = []stnrv1a1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "plaintext", auth.Type, "auth-type")
				assert.Equal(t, "ext-testuser", auth.Credentials["username"],
					"username")
				assert.Equal(t, "ext-testpass", auth.Credentials["password"],
					"password")
			},
		},
		{
			name: "mixed inline/external auth errs",
			cls:  []gwapiv1a2.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrv1a1.GatewayConfig{testutils.TestGwConfig},
			prep: func(c *renderTestConfig) {
				// gateway-config contains pass
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.AuthRef = &gwapiv1b1.SecretObjectReference{
					Name: gwapiv1b1.ObjectName("dummy-secret"),
				}
				w.Spec.AuthType = nil
				pwd := "ext-testpass"
				w.Spec.Password = &pwd
				c.cfs = []stnrv1a1.GatewayConfig{*w}

				// secret contains type and  username
				s := testutils.TestAuthSecret.DeepCopy()
				delete(s.Data, "password")
				delete(s.Data, "secret")
				c.ascrts = []corev1.Secret{*s}

			},
			tester: func(t *testing.T, r *Renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: logr.Discard()}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "mixed inline/external auth")
			},
		},
	})
}
