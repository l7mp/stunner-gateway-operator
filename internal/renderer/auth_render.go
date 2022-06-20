package renderer

import (
	"fmt"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func (r *Renderer) renderAuth(gwConf *stunnerv1alpha1.GatewayConfig) (*stunnerconfv1alpha1.AuthConfig, error) {
	r.log.V(4).Info("renderAuth", "gateway-config", store.GetObjectKey(gwConf))

	realm := stunnerconfv1alpha1.DefaultRealm
	if gwConf.Spec.Realm != nil {
		realm = *gwConf.Spec.Realm
	}

	auth := stunnerconfv1alpha1.AuthConfig{
		Realm:       realm,
		Credentials: make(map[string]string),
	}

	// FIXME auth-type validation/parsing should be provided by the stunner API
	authType := stunnerconfv1alpha1.DefaultAuthType
	if gwConf.Spec.AuthType != nil {
		authType = *gwConf.Spec.AuthType
	}

	atype, err := stunnerconfv1alpha1.NewAuthType(authType)
	if err != nil {
		return nil, err
	}
	switch atype {
	case stunnerconfv1alpha1.AuthTypePlainText:
		if gwConf.Spec.Username == nil || gwConf.Spec.Password == nil {
			return nil, fmt.Errorf("missing username and password for authetication type %q",
				"plaintext")
		}

		auth.Credentials["username"] = *gwConf.Spec.Username
		auth.Credentials["password"] = *gwConf.Spec.Password
	case stunnerconfv1alpha1.AuthTypeLongTerm:
		if gwConf.Spec.SharedSecret == nil {
			return nil, fmt.Errorf("missing shared-secret for authetication type %q",
				"longterm")
		}
		auth.Credentials["secret"] = *gwConf.Spec.SharedSecret
	}

	auth.Type = atype.String()

	// validate so that defaults get filled in
	if err = auth.Validate(); err != nil {
		return nil, err
	}

	r.log.V(4).Info("renderAuth ready", "gateway-config", store.GetObjectKey(gwConf), "result",
		fmt.Sprintf("%#v", auth))

	return &auth, nil
}
