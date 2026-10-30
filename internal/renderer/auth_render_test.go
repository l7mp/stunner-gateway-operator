package renderer

import (
	// "context"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/testutils"
	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/v2/api/v1"
)

func TestRenderAuthRender(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "default auth ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "default", auth.Name, "auth name")
				assert.Equal(t, "static", auth.Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, auth.Credentials["username"],
					"username")
				assert.Equal(t, testutils.TestPassword, auth.Credentials["password"],
					"password")
			},
		},
		{
			name: "ephemeral auth ok",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.Realm = "dummy"
				*w.Spec.AuthType = "ephemeral"
				s := "dummy"
				w.Spec.SharedSecret = &s
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, "dummy", auth.Realm, "realm")
				assert.Equal(t, "ephemeral", auth.Type, "auth-type")
				assert.Equal(t, "dummy", auth.Credentials["secret"], "secret")

			},
		},
		{
			name: "wrong auth-type errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "dummy"
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "static no-username errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "static"
				w.Spec.Username = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "static no-password errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "static"
				w.Spec.Password = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "auth type alias: plaintext errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "plaintext"
				u, p := "testuser", "testpasswd"
				w.Spec.Username = &u
				w.Spec.Password = &p
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "ephemeral no-secret errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.AuthType = "ephemeral"
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "auth type alias: timewindowed errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.Realm = "dummy"
				*w.Spec.AuthType = "timewindowed"
				s := "dummy"
				w.Spec.SharedSecret = &s
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "auth type alias: longterm errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			gws:  []gwapiv1.Gateway{},
			svcs: []corev1.Service{},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				*w.Spec.Realm = "dummy"
				*w.Spec.AuthType = "longterm"
				s := "dummy"
				w.Spec.SharedSecret = &s
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "auth-type")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		// external auth tests
		{
			name:   "default external auth ok",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "static", auth.Type, "auth-type")
				assert.Equal(t, "ext-testuser", auth.Credentials["username"],
					"username")
				assert.Equal(t, "ext-testpass", auth.Credentials["password"],
					"password")
			},
		},
		{
			name:   "ephemeral external auth ok",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				s := testutils.TestAuthSecret.DeepCopy()
				s.Data["type"] = []byte("ephemeral")
				s.Data["secret"] = []byte("ext-secret")
				c.ascrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "ephemeral", auth.Type, "auth-type")
				assert.Equal(t, "ext-secret", auth.Credentials["secret"],
					"secret")
			},
		},
		{
			name:   "ephemeral external auth with the sharedSecret key ok",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				s := testutils.TestAuthSecret.DeepCopy()
				s.Data["type"] = []byte("ephemeral")
				delete(s.Data, "secret")
				s.Data["sharedSecret"] = []byte("ext-shared-secret")
				c.ascrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")
				assert.Equal(t, "ephemeral", auth.Type, "auth-type")
				assert.Equal(t, "ext-shared-secret", auth.Credentials["secret"], "secret")
			},
		},
		{
			name:   "external auth Secret of a non-Opaque type is used anyway",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				s := testutils.TestAuthSecret.DeepCopy()
				s.Type = corev1.SecretTypeTLS
				c.ascrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")
				assert.Equal(t, "static", auth.Type, "auth-type")
				assert.Equal(t, "ext-testuser", auth.Credentials["username"], "username")
			},
		},
		{
			name:   "external auth Secret with an invalid type errs",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				s := testutils.TestAuthSecret.DeepCopy()
				s.Data["type"] = []byte("dummy")
				c.ascrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.Nil(t, auth, "no auth")
				assert.True(t, IsCritical(err), "critical err")
				assert.Equal(t, "invalid authentication type", err.Error(), "message")
			},
		},
		{
			name:   "static external auth Secret without a password errs",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				s := testutils.TestAuthSecret.DeepCopy()
				delete(s.Data, "password")
				c.ascrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.Nil(t, auth, "no auth")
				assert.True(t, IsCritical(err), "critical err")
				assert.Equal(t, "missing username and/or password for static authentication", err.Error(), "message")
			},
		},
		{
			name:   "ephemeral external auth Secret without a secret errs",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				s := testutils.TestAuthSecret.DeepCopy()
				s.Data["type"] = []byte("ephemeral")
				delete(s.Data, "secret")
				c.ascrts = []corev1.Secret{*s}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.Nil(t, auth, "no auth")
				assert.True(t, IsCritical(err), "critical err")
				assert.Equal(t, "missing shared-secret for ephemeral authentication", err.Error(), "message")
			},
		},
		{
			name:   "wrong secret group errs",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				group := gwapiv1.Group("dummy-group")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Group: &group,
					Name:  gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "renderAuth")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name:   "wrong secret kind errs",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				kind := gwapiv1.Kind("dummy-kind")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Kind: &kind,
					Name: gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "renderAuth")
				assert.True(t, IsCritical(err), "critical err")
			},
		},
		{
			name: "missing secret errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				namespace := gwapiv1.Namespace("testnamespace")
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Namespace: &namespace,
					Name:      gwapiv1.ObjectName("dummy-secret"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "missing secret")
				assert.Equal(t, "missing or invalid external authentication credentials", err.Error(), "message")
			},
		},
		{
			name:   "missing namespace ok",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				// add AuthRef to gwconf and remove inline auth
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Name: gwapiv1.ObjectName("testauthsecret-ok"),
				}
				w.Spec.AuthType = nil
				w.Spec.Username = nil
				w.Spec.Password = nil
				w.Spec.SharedSecret = nil
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "static", auth.Type, "auth-type")
				assert.Equal(t, "ext-testuser", auth.Credentials["username"],
					"username")
				assert.Equal(t, "ext-testpass", auth.Credentials["password"],
					"password")
			},
		},
		{
			name:   "external auth overrides inline",
			cls:    []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:    []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			ascrts: []corev1.Secret{testutils.TestAuthSecret},
			prep: func(c *renderTestConfig) {
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Name: gwapiv1.ObjectName("testauthsecret-ok"),
				}
				atype := "ephemeral"
				sharedSecret := "testsecret"
				w.Spec.AuthType = &atype
				w.Spec.SharedSecret = &sharedSecret
				c.cfs = []stnrgwv1.GatewayConfig{*w}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")

				assert.Equal(t, testutils.TestRealm, auth.Realm, "realm")
				assert.Equal(t, "static", auth.Type, "auth-type")
				assert.Equal(t, "ext-testuser", auth.Credentials["username"],
					"username")
				assert.Equal(t, "ext-testpass", auth.Credentials["password"],
					"password")
			},
		},
		{
			name: "mixed inline/external auth errs",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			prep: func(c *renderTestConfig) {
				// gateway-config contains pass
				w := testutils.TestGwConfig.DeepCopy()
				w.Spec.AuthRef = &gwapiv1.SecretObjectReference{
					Name: gwapiv1.ObjectName("dummy-secret"),
				}
				w.Spec.AuthType = nil
				pwd := "ext-testpass"
				w.Spec.Password = &pwd
				c.cfs = []stnrgwv1.GatewayConfig{*w}

				// secret contains type and  username
				s := testutils.TestAuthSecret.DeepCopy()
				delete(s.Data, "password")
				delete(s.Data, "secret")
				c.ascrts = []corev1.Secret{*s}

			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				_, err = r.renderAuth(c)
				assert.Error(t, err, "mixed inline/external auth")
			},
		},
	})
}

func TestRenderAuthSTUNMode(t *testing.T) {
	renderTester(t, []renderTestConfig{
		{
			name: "STUN mode renders no credentials",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			prep: func(c *renderTestConfig) {
				gwc := testutils.TestGwConfig.DeepCopy()
				gwc.Spec.STUNMode = true
				empty := ""
				gwc.Spec.Realm = &empty
				gwc.Spec.AuthType = &empty
				gwc.Spec.Username = &empty
				gwc.Spec.Password = &empty
				c.cfs = []stnrgwv1.GatewayConfig{*gwc}
			},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")
				assert.Equal(t, stnrapiv2.DefaultRealm, auth.Realm, "realm")
				assert.Equal(t, "none", auth.Type, "auth-type")
				assert.Empty(t, auth.Credentials, "no credentials")
			},
		},
		{
			name: "STUN mode off renders the TURN credentials",
			cls:  []gwapiv1.GatewayClass{testutils.TestGwClass},
			cfs:  []stnrgwv1.GatewayConfig{testutils.TestGwConfig},
			prep: func(c *renderTestConfig) {},
			tester: func(t *testing.T, r *renderer) {
				gc, err := r.getGatewayClass()
				assert.NoError(t, err, "gw-class found")
				c := &RenderContext{gc: gc, log: log}
				c.gwConf, err = r.getGatewayConfig4Class(c)
				assert.NoError(t, err, "gw-conf found")

				auth, err := r.renderAuth(c)
				assert.NoError(t, err, "renderAuth")
				assert.Equal(t, "static", auth.Type, "auth-type")
				assert.Equal(t, testutils.TestUsername, auth.Credentials["username"], "username")
			},
		},
	})
}
