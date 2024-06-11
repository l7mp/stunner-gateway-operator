package renderer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) renderAuth(c *RenderContext) (*stnrconfv1.AuthConfig, error) {
	// external auth ref overrides inline refs
	if c.gwConf.Spec.AuthRef != nil {
		return r.renderExternalAuth(c)
	}

	return r.renderInlineAuth(c)
}

func (r *Renderer) renderInlineAuth(c *RenderContext) (*stnrconfv1.AuthConfig, error) {
	realm := stnrconfv1.DefaultRealm
	if c.gwConf.Spec.Realm != nil {
		realm = *c.gwConf.Spec.Realm
	}

	auth := stnrconfv1.AuthConfig{
		Realm:       realm,
		Credentials: make(map[string]string),
	}

	atype, err := getAuthType(c.gwConf.Spec.AuthType)
	if err != nil {
		return nil, err
	}

	switch atype {
	case stnrconfv1.AuthTypePlainText:
		if c.gwConf.Spec.Username == nil || c.gwConf.Spec.Password == nil {
			return nil, NewCriticalError(InvalidUsernamePassword)
		}

		auth.Credentials["username"] = *c.gwConf.Spec.Username
		auth.Credentials["password"] = *c.gwConf.Spec.Password

	case stnrconfv1.AuthTypeLongTerm:
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

	r.log.V(2).Info("Rendering inline auth config ready", "gateway-config",
		store.GetObjectKey(c.gwConf), "result", fmt.Sprintf("%#v", auth))

	return &auth, nil
}

func (r *Renderer) renderExternalAuth(c *RenderContext) (*stnrconfv1.AuthConfig, error) {
	gwConf := c.gwConf
	// r.log.V(4).Info("renderExternalAuth", "gateway-config", store.GetObjectKey(gwConf))

	realm := stnrconfv1.DefaultRealm
	if gwConf.Spec.Realm != nil {
		realm = *gwConf.Spec.Realm
	}

	auth := stnrconfv1.AuthConfig{
		Realm:       realm,
		Credentials: make(map[string]string),
	}

	ref := c.gwConf.Spec.AuthRef
	n, err := getSecretNameFromRef(ref, gwConf.GetNamespace())
	if err != nil {
		// report concrete error here, return a critical error
		r.log.Info("Invalid auth Secret", "gateway-config", store.GetObjectKey(c.gwConf),
			"ref", dumpSecretRef(ref, gwConf.GetNamespace()), "error", err.Error())
		return nil, NewCriticalError(ExternalAuthCredentialsNotFound)
	}

	secret := store.AuthSecrets.GetObject(n)
	if secret == nil {
		// report concrete error here, return a critical error
		r.log.Info("Auth Secret not found", "gateway-config", store.GetObjectKey(c.gwConf),
			"ref", dumpSecretRef(ref, gwConf.GetNamespace()), "name", n)
		return nil, NewCriticalError(ExternalAuthCredentialsNotFound)
	}

	if secret.Type != corev1.SecretTypeOpaque {
		r.log.Info("Expecting Secret of type \"Opaque\" (trying to use Secret anyway)",
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
	case stnrconfv1.AuthTypePlainText:
		username, usernameOk := secret.Data["username"]
		password, passwordOk := secret.Data["password"]

		if !usernameOk || !passwordOk {
			return nil, NewCriticalError(InvalidUsernamePassword)
		}

		auth.Credentials["username"] = string(username)
		auth.Credentials["password"] = string(password)

	case stnrconfv1.AuthTypeLongTerm:
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

	r.log.V(2).Info("Finished rendering external auth config", "gateway-config", store.GetObjectKey(gwConf),
		"secret", n.String(), "result", fmt.Sprintf("%#v", auth))

	return &auth, nil
}

func getAuthType(hint *string) (stnrconfv1.AuthType, error) {
	authType := stnrconfv1.DefaultAuthType
	if hint != nil {
		authType = *hint
	}

	// aliases
	switch authType {
	// plaintext
	case "static", "plaintext":
		authType = "plaintext"
	case "ephemeral", "timewindowed", "longterm":
		authType = "longterm"
	}

	atype, err := stnrconfv1.NewAuthType(authType)
	if err != nil {
		return stnrconfv1.AuthTypeUnknown, NewCriticalError(InvalidAuthType)
	}

	return atype, nil
}
