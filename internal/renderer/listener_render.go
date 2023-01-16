package renderer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stnrconfv1a1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	stnrv1a1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func stnrListenerName(gw *gwapiv1a2.Gateway, l *gwapiv1a2.Listener) string {
	return fmt.Sprintf("%s/%s", store.GetObjectKey(gw), string(l.Name))
}

func (r *Renderer) renderListener(gw *gwapiv1a2.Gateway, gwConf *stnrv1a1.GatewayConfig, l *gwapiv1a2.Listener, rs []*gwapiv1a2.UDPRoute, ap *addrPort) (*stnrconfv1a1.ListenerConfig, error) {
	r.log.V(4).Info("renderListener", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "public-addr",
		fmt.Sprintf("%#v", ap))

	proto, err := stnrconfv1a1.NewListenerProtocol(string(l.Protocol))
	if err != nil {
		return nil, err
	}

	minPort, maxPort := stnrconfv1a1.DefaultMinRelayPort,
		stnrconfv1a1.DefaultMaxRelayPort
	if gwConf.Spec.MinPort != nil {
		minPort = int(*gwConf.Spec.MinPort)
	}
	if gwConf.Spec.MaxPort != nil {
		maxPort = int(*gwConf.Spec.MaxPort)
	}

	a, p := "", 0
	if ap != nil {
		a = ap.addr
		p = ap.port
	}

	lc := stnrconfv1a1.ListenerConfig{
		Name:         stnrListenerName(gw, l),
		Protocol:     proto.String(),
		Addr:         "$STUNNER_ADDR", // Addr will be filled in from the pod environment
		Port:         int(l.Port),
		PublicAddr:   a,
		PublicPort:   p,
		MinRelayPort: minPort,
		MaxRelayPort: maxPort,
	}

	if cert, key, ok := r.getTLS(gw, l); ok {
		lc.Cert = cert
		lc.Key = key
	}

	for _, r := range rs {
		lc.Routes = append(lc.Routes, store.GetObjectKey(r))
	}

	r.log.V(2).Info("renderListener ready", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "result",
		fmt.Sprintf("%#v", lc))

	return &lc, nil
}

func (r *Renderer) getTLS(gw *gwapiv1a2.Gateway, l *gwapiv1a2.Listener) (string, string, bool) {
	proto, _ := stnrconfv1a1.NewListenerProtocol(string(l.Protocol))

	if l.TLS == nil || (l.TLS.Mode != nil && *l.TLS.Mode != gwapiv1a2.TLSModeTerminate) ||
		(proto != stnrconfv1a1.ListenerProtocolTLS && proto != stnrconfv1a1.ListenerProtocolDTLS) {
		return "", "", false
	}

	if len(l.TLS.CertificateRefs) == 0 {
		r.log.Info("no CertificateRef found in Gateway listener", "gateway", store.GetObjectKey(gw),
			"listener", l.Name)
		return "", "", false
	}

	if len(l.TLS.CertificateRefs) > 1 {
		r.log.Info("too many CertificateRef found in Gateway listener, using the first one",
			"gateway", store.GetObjectKey(gw), "listener", l.Name)
	}

	for _, ref := range l.TLS.CertificateRefs {
		ref := ref
		if (ref.Group != nil && *ref.Group != corev1.GroupName) ||
			(ref.Kind != nil && *ref.Kind != "Secret") {
			name := ""
			if ref.Group != nil {
				name = string(*ref.Group)
			}
			if ref.Kind != nil {
				name = fmt.Sprintf("%s/%s", name, string(*ref.Kind))
			}

			r.log.Info("ignoring secret-reference to an unknown object", "gateway", store.GetObjectKey(gw),
				"listener", l.Name, "object-ref", name)
			continue
		}

		// find the named secret, use the Gw namespace if no namespace found in the ref
		namespace := gw.GetNamespace()
		if ref.Namespace != nil {
			namespace = string(*ref.Namespace)
		}

		n := types.NamespacedName{Namespace: namespace, Name: string(ref.Name)}
		secret := store.Secrets.GetObject(n)
		if secret == nil {
			r.log.Info("secret not found", "gateway", store.GetObjectKey(gw),
				"listener", l.Name, "secret", n.String())
			// fall through: we may find another workable cert-ref
			continue
		}

		if secret.Type != corev1.SecretTypeTLS {
			r.log.Info("expecting Secret of type \"kubernetes.io/tls\" (trying to "+
				"use Secret anyway)", "gateway", store.GetObjectKey(gw), "listener",
				l.Name, "secret", n.String())
		}

		// make this foolproof
		cert, certOk := secret.Data["tls.crt"]
		if !certOk {
			cert, certOk = secret.Data["crt"]
			if !certOk {
				cert, certOk = secret.Data["cert"]
			}
		}

		key, keyOk := secret.Data["tls.key"]
		if !keyOk {
			key, keyOk = secret.Data["key"]
		}

		if !certOk || !keyOk {
			r.log.Info("cannot find cert and/or key in Secret", "gateway",
				store.GetObjectKey(gw), "listener", l.Name, "secret", n.String())
			continue
		}

		return string(cert), string(key), true
	}

	return "", "", false
}
