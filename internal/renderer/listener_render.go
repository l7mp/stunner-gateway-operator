package renderer

import (
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/go-logr/logr"
	stnrconfv1 "github.com/l7mp/stunner/pkg/apis/v1"

	"github.com/l7mp/stunner-gateway-operator/internal/store"

	stnrgwv1 "github.com/l7mp/stunner-gateway-operator/api/v1"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

func stnrListenerName(gw *gwapiv1.Gateway, l *gwapiv1.Listener) string {
	return fmt.Sprintf("%s/%s", store.GetObjectKey(gw), string(l.Name))
}

var _ configRenderer = &listenerRenderer{}

type listenerRenderer struct{ log logr.Logger }

func newListenerRenderer(log logr.Logger) configRenderer {
	return &listenerRenderer{log: log}
}

func (r *listenerRenderer) render(c *RenderContext, args ...any) (stnrconfv1.Config, error) {
	if c.gws.Len() != 1 {
		return nil, fmt.Errorf("Internal error: Invalid call to listenerRender with %d gws", c.gws.Len())
	}
	gw := c.gws.GetFirst()
	gwConf := c.gwConf

	// rest of the arguments are of type any
	if len(args) != 4 {
		return nil, fmt.Errorf("Internal error: Invalid call to listenerRender with args %v", args)
	}
	l := args[0].(*gwapiv1.Listener)
	rs := args[1].([]*stnrgwv1.UDPRoute)
	ap := args[2].(gwAddrPort)
	targetPorts := args[3].(map[string]int)

	r.log.V(6).Info("renderListener", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "public-addr", ap.String())

	proto, err := getProtocol(l.Protocol)
	if err != nil {
		return nil, err
	}

	port := int(l.Port)
	if targetPorts != nil {
		if p, ok := targetPorts[string(l.Name)]; ok {
			port = p
		}
	}

	lc := stnrconfv1.ListenerConfig{
		Name:     stnrListenerName(gw, l),
		Protocol: proto.String(),
		Addr:     opdefault.DefaultSTUNnerAddressEnvVarName, // $STUNNER_ADDR, will be filled in from the pod environment
		Port:     port,
	}

	// set public address-port
	if ap.addr != "" && ap.port > 0 {
		lc.PublicAddr = ap.addr
		lc.PublicPort = ap.port
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

	r.log.V(2).Info("Finished rendering listener config", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "result",
		fmt.Sprintf("%#v", tmp))

	return &lc, nil
}

func (r *listenerRenderer) getTLS(gw *gwapiv1.Gateway, l *gwapiv1.Listener) (string, string, bool) {
	proto, err := getProtocol(l.Protocol)
	if err != nil {
		return "", "", false
	}

	if l.TLS == nil || (l.TLS.Mode != nil && *l.TLS.Mode != gwapiv1.TLSModeTerminate) ||
		(proto != stnrconfv1.ListenerProtocolTURNTLS && proto != stnrconfv1.ListenerProtocolTURNDTLS) {
		return "", "", false
	}

	if len(l.TLS.CertificateRefs) == 0 {
		r.log.Info("No certificate reference found in Gateway listener", "gateway", store.GetObjectKey(gw),
			"listener", l.Name)
		return "", "", false
	}

	if len(l.TLS.CertificateRefs) > 1 {
		r.log.Info("Too many certificate references found in Gateway listener, using the first one",
			"gateway", store.GetObjectKey(gw), "listener", l.Name)
	}

	for _, ref := range l.TLS.CertificateRefs {
		ref := ref

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

		return base64.StdEncoding.EncodeToString(cert),
			base64.StdEncoding.EncodeToString(key), true
	}

	return "", "", false
}

// normalize protocol aliases
func getProtocol(proto gwapiv1.ProtocolType) (stnrconfv1.ListenerProtocol, error) {
	protocol := string(proto)
	switch protocol {
	case "UDP":
		protocol = "TURN-UDP" // v0.16: resolves to TURN-UDP

	case "TCP":
		protocol = "TURN-TCP" // v0.16: resolves to TURN-TCP

	case "TLS":
		protocol = "TURN-TLS" // v0.16: resolves to TURN-TLS

	case "DTLS":
		protocol = "TURN-DTLS" // v0.16: resolves to TURN-DTLS
	}

	ret, err := stnrconfv1.NewListenerProtocol(protocol)
	if err != nil {
		return ret, NewNonCriticalError(InvalidProtocol)
	}

	return ret, nil
}
