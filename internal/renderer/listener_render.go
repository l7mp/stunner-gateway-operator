package renderer

import (
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	gwapiv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwapiv1b1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
)

func stnrListenerName(gw *gwapiv1b1.Gateway, l *gwapiv1b1.Listener) string {
	return fmt.Sprintf("%s/%s", store.GetObjectKey(gw), string(l.Name))
}

func (r *Renderer) renderListener(gw *gwapiv1b1.Gateway, gwConf *stnrgwv1.GatewayConfig, l *gwapiv1b1.Listener, rs []*gwapiv1a2.UDPRoute, ap *gatewayAddress) (*stnrconfv1.ListenerConfig, error) {
	r.log.V(4).Info("renderListener", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "public-addr", ap.String())

	proto, err := r.getProtocol(l.Protocol)
	if err != nil {
		return nil, err
	}

	// minPort, maxPort := stnrconfv1.DefaultMinRelayPort,
	// 	stnrconfv1.DefaultMaxRelayPort
	// if gwConf.Spec.MinPort != nil {
	// 	minPort = int(*gwConf.Spec.MinPort)
	// }
	// if gwConf.Spec.MaxPort != nil {
	// 	maxPort = int(*gwConf.Spec.MaxPort)
	// }

	a, p := "", 0
	if ap != nil {
		a = ap.addr
		p = ap.port
	}

	lc := stnrconfv1.ListenerConfig{
		Name:         stnrListenerName(gw, l),
		Protocol:     proto.String(),
		Addr:         "$STUNNER_ADDR", // Addr will be filled in from the pod environment
		Port:         int(l.Port),
		PublicAddr:   a,
		PublicPort:   p,
		MinRelayPort: stnrconfv1.DefaultMinRelayPort,
		MaxRelayPort: stnrconfv1.DefaultMaxRelayPort,
	}

	if cert, key, ok := r.getTLS(gw, l); ok {
		lc.Cert = cert
		lc.Key = key
	}

	for _, r := range rs {
		lc.Routes = append(lc.Routes, store.GetObjectKey(r))
	}

	// remove cert/key from dump
	tmp := lc
	tmp.Cert = "<SECRET>"
	tmp.Key = "<SECRET>"

	r.log.V(2).Info("renderListener ready", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "result",
		fmt.Sprintf("%#v", tmp))

	return &lc, nil
}

func (r *Renderer) getTLS(gw *gwapiv1b1.Gateway, l *gwapiv1b1.Listener) (string, string, bool) {
	proto, err := r.getProtocol(l.Protocol)
	if err != nil {
		return "", "", false
	}

	if l.TLS == nil || (l.TLS.Mode != nil && *l.TLS.Mode != gwapiv1b1.TLSModeTerminate) ||
		(proto != stnrconfv1.ListenerProtocolTURNTLS && proto != stnrconfv1.ListenerProtocolTURNDTLS) {
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

		n, err := getSecretNameFromRef(&ref, gw.GetNamespace())
		if err != nil {
			r.log.Info("ignoring secret-reference to an unknown or invalid object",
				"gateway", store.GetObjectKey(gw),
				"ref", dumpSecretRef(&ref, gw.GetNamespace()),
				"error", err.Error())
			continue
		}

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

		return base64.StdEncoding.EncodeToString(cert),
			base64.StdEncoding.EncodeToString(key), true
	}

	return "", "", false
}

// normalize protocol aliases
func (r *Renderer) getProtocol(proto gwapiv1b1.ProtocolType) (stnrconfv1.ListenerProtocol, error) {
	protocol := string(proto)
	switch protocol {
	case "UDP":
		protocol = "TURN-UDP" // v0.16: resolves to TURN-UDP
		r.log.Info("use of DEPRECATED protocol", "deprecated-protocol", "UDP",
			"valid-protocol", "TURN-UDP")
	case "TCP":
		protocol = "TURN-TCP" // v0.16: resolves to TURN-TCP
		r.log.Info("use of DEPRECATED protocol", "deprecated-protocol", "TCP",
			"valid-protocol", "TURN-TCP")
	case "TLS":
		protocol = "TURN-TLS" // v0.16: resolves to TURN-TLS
		r.log.Info("use of DEPRECATED protocol", "deprecated-protocol", "TLS",
			"valid-protocol", "TURN-TLS")
	case "DTLS":
		protocol = "TURN-DTLS" // v0.16: resolves to TURN-DTLS
		r.log.Info("use of DEPRECATED protocol", "deprecated-protocol", "DTLS",
			"valid-protocol", "TURN-DTLS")
	}

	ret, err := stnrconfv1.NewListenerProtocol(protocol)
	if err != nil {
		return ret, NewNonCriticalError(InvalidProtocol)
	}

	return ret, nil
}
