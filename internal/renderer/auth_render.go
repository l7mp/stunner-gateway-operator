package renderer

import (
	"fmt"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) renderAuth(c *RenderContext) (*stnrconfv1a1.AuthConfig, error) {
	gwConf := c.gwConf
	r.log.V(4).Info("renderAuth", "gateway-config", store.GetObjectKey(gwConf))

	realm := stnrconfv1a1.DefaultRealm
	if gwConf.Spec.Realm != nil {
		realm = *gwConf.Spec.Realm
	}

	auth := stnrconfv1a1.AuthConfig{
		Realm:       realm,
		Credentials: make(map[string]string),
	}

	// FIXME auth-type validation/parsing should be provided by the stunner API
	authType := stnrconfv1a1.DefaultAuthType
	if gwConf.Spec.AuthType != nil {
		authType = *gwConf.Spec.AuthType
	}

	atype, err := stnrconfv1a1.NewAuthType(authType)
	if err != nil {
		return nil, err
	}
	switch atype {
	case stnrconfv1a1.AuthTypePlainText:
		if gwConf.Spec.Username == nil || gwConf.Spec.Password == nil {
			return nil, NewNonCriticalError(InvalidUsernamePassword)
		}

		auth.Credentials["username"] = *gwConf.Spec.Username
		auth.Credentials["password"] = *gwConf.Spec.Password

	case stnrconfv1a1.AuthTypeLongTerm:
		if gwConf.Spec.SharedSecret == nil {
			return nil, NewNonCriticalError(InvalidUsernamePassword)
		}
		auth.Credentials["secret"] = *gwConf.Spec.SharedSecret
	}

	auth.Type = atype.String()

	// validate so that defaults get filled in
	if err = auth.Validate(); err != nil {
		return nil, err
	}

	r.log.V(2).Info("renderAuth ready", "gateway-config", store.GetObjectKey(gwConf), "result",
		fmt.Sprintf("%#v", auth))

	return &auth, nil
}
