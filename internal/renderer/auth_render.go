package renderer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
)

var _ configRenderer = &authRenderer{}

type authRenderer struct{}

func newAuthRenderer() configRenderer {
	return &authRenderer{}
}

func (r *authRenderer) render(c *RenderContext, _ ...any) (stnrapiv2.Config, error) {
	// STUN-server mode: no TURN allocations, whatever credentials the GatewayConfig holds
	if c.gwConf.Spec.STUNMode {
		realm := stnrapiv2.DefaultRealm
		if c.gwConf.Spec.Realm != nil {
			realm = *c.gwConf.Spec.Realm
		}
		auth := stnrapiv2.AuthConfig{Name: stnrapiv2.DefaultAuthName, Realm: realm, Type: "none"}
		if err := auth.Validate(); err != nil {
			return nil, NewCriticalError(InvalidAuthConfig)
		}
		return &auth, nil
	}

	// external auth ref overrides inline refs
	if c.gwConf.Spec.AuthRef != nil {
		return r.renderExternalAuth(c)
	}

	return r.renderInlineAuth(c)
}

func (r *authRenderer) renderInlineAuth(c *RenderContext) (stnrapiv2.Config, error) {
	realm := stnrapiv2.DefaultRealm
	if c.gwConf.Spec.Realm != nil {
		realm = *c.gwConf.Spec.Realm
	}

	auth := stnrapiv2.AuthConfig{
		Name:        stnrapiv2.DefaultAuthName,
		Realm:       realm,
		Credentials: make(map[string]string),
	}

	atype, err := getAuthType(c.gwConf.Spec.AuthType)
	if err != nil {
		return nil, err
	}

	switch atype {
	case stnrapiv2.AuthTypeStatic:
		if c.gwConf.Spec.Username == nil || c.gwConf.Spec.Password == nil {
			return nil, NewCriticalError(InvalidUsernamePassword)
		}

		auth.Credentials["username"] = *c.gwConf.Spec.Username
		auth.Credentials["password"] = *c.gwConf.Spec.Password

	case stnrapiv2.AuthTypeEphemeral:
		if c.gwConf.Spec.SharedSecret == nil {
			return nil, NewCriticalError(InvalidSharedSecret)
		}
		auth.Credentials["secret"] = *c.gwConf.Spec.SharedSecret
	}

	auth.Type = atype.String()

	// validate so that defaults get filled in
	if err = auth.Validate(); err != nil {
		return nil, NewCriticalError(InvalidAuthConfig)
	}

	c.log.V(2).Info("Rendering inline auth config ready", "gateway-config",
		store.GetObjectKey(c.gwConf), "result", fmt.Sprintf("%#v", auth))

	return &auth, nil
}

func (r *authRenderer) renderExternalAuth(c *RenderContext) (stnrapiv2.Config, error) {
	gwConf := c.gwConf

	realm := stnrapiv2.DefaultRealm
	if gwConf.Spec.Realm != nil {
		realm = *gwConf.Spec.Realm
	}

	auth := stnrapiv2.AuthConfig{
		Name:        stnrapiv2.DefaultAuthName,
		Realm:       realm,
		Credentials: make(map[string]string),
	}

	ref := c.gwConf.Spec.AuthRef
	n, err := getSecretNameFromRef(ref, gwConf.GetNamespace())
	if err != nil {
		// report concrete error here, return a critical error
		c.log.Info("Invalid auth Secret", "gateway-config", store.GetObjectKey(c.gwConf),
			"ref", dumpSecretRef(ref, gwConf.GetNamespace()), "error", err.Error())
		return nil, NewCriticalError(ExternalAuthCredentialsNotFound)
	}

	secret := store.AuthSecrets.GetObject(n)
	if secret == nil {
		// report concrete error here, return a critical error
		c.log.Info("Auth Secret not found", "gateway-config", store.GetObjectKey(c.gwConf),
			"ref", dumpSecretRef(ref, gwConf.GetNamespace()), "name", n)
		return nil, NewCriticalError(ExternalAuthCredentialsNotFound)
	}

	if secret.Type != corev1.SecretTypeOpaque {
		c.log.Info("Expecting Secret of type \"Opaque\" (trying to use Secret anyway)",
			"gateway-config", store.GetObjectKey(c.gwConf), "secret", n.String())
	}

	var hint *string
	if stype, ok := secret.Data["type"]; ok {
		stype := string(stype)
		hint = &stype
	}

	atype, err := getAuthType(hint)
	if err != nil {
		return nil, err
	}

	switch atype {
	case stnrapiv2.AuthTypeStatic:
		username, usernameOk := secret.Data["username"]
		password, passwordOk := secret.Data["password"]

		if !usernameOk || !passwordOk {
			return nil, NewCriticalError(InvalidUsernamePassword)
		}

		auth.Credentials["username"] = string(username)
		auth.Credentials["password"] = string(password)

	case stnrapiv2.AuthTypeEphemeral:
		sharedSecret, sharedSecretOk := secret.Data["secret"]
		// accept long form
		if !sharedSecretOk {
			sharedSecret, sharedSecretOk = secret.Data["sharedSecret"]
		}

		if !sharedSecretOk {
			return nil, NewCriticalError(InvalidSharedSecret)
		}

		auth.Credentials["secret"] = string(sharedSecret)
	}

	auth.Type = atype.String()

	// validate so that defaults get filled in
	if err = auth.Validate(); err != nil {
		return nil, NewCriticalError(InvalidAuthConfig)
	}

	c.log.V(2).Info("Finished rendering external auth config", "gateway-config", store.GetObjectKey(gwConf),
		"secret", n.String(), "result", fmt.Sprintf("%#v", auth))

	return &auth, nil
}

func getAuthType(hint *string) (stnrapiv2.AuthType, error) {
	authType := stnrapiv2.DefaultAuthType
	if hint != nil {
		authType = *hint
	}

	atype, err := stnrapiv2.NewAuthType(authType)
	if err != nil {
		return stnrapiv2.AuthTypeNone, NewCriticalError(InvalidAuthType)
	}

	return atype, nil
}

// renderTunnelAuth renders the authentication policy the tunnel of a TURNRoute presents to the
// upstream TURN server, named after the cluster of the route, from the Secret the route references:
// the key "type", the optional keys "realm" and "lifetime", and every other key a credential. It
// returns nil for any other route and for a TURNRoute referencing no Secret.
func renderTunnelAuth(ro store.Route) (*stnrapiv2.AuthConfig, error) {
	tr, ok := ro.(*stnrgwv1a1.TURNRoute)
	if !ok || tr.Spec.Tunnel.AuthRef == nil {
		return nil, nil
	}
	n, err := getSecretNameFromRef(tr.Spec.Tunnel.AuthRef, tr.GetNamespace())
	if err != nil {
		return nil, NewNonCriticalError(TunnelAuthNotFound)
	}
	secret := store.TunnelAuthSecrets.GetObject(n)
	if secret == nil {
		return nil, NewNonCriticalError(TunnelAuthNotFound)
	}

	auth := stnrapiv2.AuthConfig{Name: clusterName(ro), Credentials: map[string]string{}}
	for k, v := range secret.Data {
		switch k {
		case "type":
			auth.Type = string(v)
		case "realm":
			auth.Realm = string(v)
		case "lifetime":
			auth.Lifetime = string(v)
		default:
			auth.Credentials[k] = string(v)
		}
	}
	if err := auth.Validate(); err != nil {
		return nil, NewNonCriticalError(InvalidTunnelAuth)
	}
	return &auth, nil
}
