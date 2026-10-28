package renderer

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"

	"github.com/l7mp/stunner-gateway-operator/v2/internal/store"
)

func stnrListenerName(gw *gwapiv1.Gateway, l *gwapiv1.Listener) string {
	return fmt.Sprintf("%s/%s", store.GetObjectKey(gw), string(l.Name))
}

var _ listenerRenderer = &defaultListenerRenderer{}

type defaultListenerRenderer struct{ log logr.Logger }

func newListenerRenderer(log logr.Logger) listenerRenderer {
	return &defaultListenerRenderer{log: log}
}

// renderListener renders a Gateway listener into a dataplane listener feeding the server named
// after the listener.
func (r *defaultListenerRenderer) renderListener(c *RenderContext, l *gwapiv1.Listener, rs []store.Route, ap gwAddrPort, targetPorts map[string]int) (*stnrapiv2.ListenerConfig, error) {
	if c.gws.Len() != 1 {
		return nil, fmt.Errorf("Internal error: Invalid call to renderListener with %d gws", c.gws.Len())
	}

	gw := c.gws.GetFirst()
	gwConf := c.gwConf

	r.log.V(6).Info("renderListener", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "public-addr", ap.String())

	proto, _, err := getProtocol(l.Protocol)
	if err != nil {
		return nil, err
	}

	port := int(l.Port)
	if targetPorts != nil {
		if p, ok := targetPorts[string(l.Name)]; ok {
			port = p
		}
	}

	name := stnrListenerName(gw, l)
	lc := stnrapiv2.ListenerConfig{
		Name:     name,
		Protocol: proto.String(),
		Servers:  []string{name},
		Port:     port,
	}

	// set public address-port
	if ap.addr != "" && ap.port > 0 {
		lc.PublicAddr = ap.addr
		lc.PublicPort = ap.port
	}

	cert, key, ok, err := r.getTLS(gw, l)
	if err != nil {
		return nil, err
	}

	if ok {
		lc.TLS = &stnrapiv2.TLSConfig{Cert: cert, Key: key}
	}

	// remove cert/key from dump
	tmp := lc
	if lc.TLS != nil {
		t := *lc.TLS
		t.Cert, t.Key = "<SECRET>", "<SECRET>"
		tmp.TLS = &t
	}

	r.log.V(2).Info("Finished rendering listener config", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "result",
		fmt.Sprintf("%#v", tmp))

	return &lc, nil
}

func (r *defaultListenerRenderer) getTLS(gw *gwapiv1.Gateway, l *gwapiv1.Listener) (string, string, bool, error) {
	proto, _, err := getProtocol(l.Protocol)
	if err != nil {
		return "", "", false, nil
	}

	if l.TLS == nil || (l.TLS.Mode != nil && *l.TLS.Mode != gwapiv1.TLSModeTerminate) ||
		(proto != stnrapiv2.ProtocolTLS && proto != stnrapiv2.ProtocolDTLS) {
		return "", "", false, nil
	}

	if len(l.TLS.CertificateRefs) == 0 {
		r.log.Info("No certificate reference found in Gateway listener", "gateway", store.GetObjectKey(gw),
			"listener", l.Name)
		return "", "", false, nil
	}

	if len(l.TLS.CertificateRefs) > 1 {
		r.log.Info("Too many certificate references found in Gateway listener, using the first one",
			"gateway", store.GetObjectKey(gw), "listener", l.Name)
	}

	for _, ref := range l.TLS.CertificateRefs {

		n, err := getSecretNameFromRef(&ref, gw.GetNamespace())
		if err != nil {
			r.log.Info("Ignoring secret-reference to an unknown or invalid object",
				"gateway", store.GetObjectKey(gw),
				"ref", dumpSecretRef(&ref, gw.GetNamespace()),
				"error", err.Error())
			continue
		}

		secret := store.TLSSecrets.GetObject(n)
		if secret == nil {
			r.log.Info("Secret not found", "gateway", store.GetObjectKey(gw),
				"listener", l.Name, "secret", n.String())
			// fall through: we may find another workable cert-ref
			continue
		}

		if secret.Type != corev1.SecretTypeTLS {
			r.log.Info("Expecting Secret of type \"kubernetes.io/tls\" (trying to "+
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
			r.log.Info("Cannot find cert and/or key in Secret", "gateway",
				store.GetObjectKey(gw), "listener", l.Name, "secret", n.String())
			continue
		}

		// stunnerd refuses a pair that does not parse
		if _, err := tls.X509KeyPair(cert, key); err != nil {
			r.log.Info("Invalid cert/key pair in Secret", "gateway", store.GetObjectKey(gw),
				"listener", l.Name, "secret", n.String(), "error", err.Error())
			continue
		}

		return base64.StdEncoding.EncodeToString(cert),
			base64.StdEncoding.EncodeToString(key), true, nil
	}

	return "", "", false, NewNonCriticalError(InvalidCertificateRef)
}

// getProtocol parses the protocol of a Gateway listener into the protocol of its dataplane listener
// and the type of the server the listener feeds: TURN-* is a TURN server over the transport
// underneath, a plain UDP, TCP, TLS or DTLS an L4 server, and SRT an SRT server over UDP.
func getProtocol(proto gwapiv1.ProtocolType) (stnrapiv2.Protocol, stnrapiv2.ServerType, error) {
	p, err := stnrapiv2.NewProtocol(string(proto))
	if err != nil {
		return stnrapiv2.ProtocolUnknown, stnrapiv2.ServerTypeUnknown, NewNonCriticalError(InvalidProtocol)
	}

	switch p {
	case stnrapiv2.ProtocolUDP, stnrapiv2.ProtocolTCP, stnrapiv2.ProtocolTLS, stnrapiv2.ProtocolDTLS:
		return p, stnrapiv2.ServerTypeL4, nil
	case stnrapiv2.ProtocolTURNUDP:
		return stnrapiv2.ProtocolUDP, stnrapiv2.ServerTypeTURN, nil
	case stnrapiv2.ProtocolTURNTCP:
		return stnrapiv2.ProtocolTCP, stnrapiv2.ServerTypeTURN, nil
	case stnrapiv2.ProtocolTURNTLS:
		return stnrapiv2.ProtocolTLS, stnrapiv2.ServerTypeTURN, nil
	case stnrapiv2.ProtocolTURNDTLS:
		return stnrapiv2.ProtocolDTLS, stnrapiv2.ServerTypeTURN, nil
	case stnrapiv2.ProtocolSRT:
		return stnrapiv2.ProtocolUDP, stnrapiv2.ServerTypeSRT, nil
	}

	return stnrapiv2.ProtocolUnknown, stnrapiv2.ServerTypeUnknown, NewNonCriticalError(InvalidProtocol)
}
