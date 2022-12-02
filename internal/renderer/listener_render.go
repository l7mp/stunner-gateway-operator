package renderer

import (
	"fmt"
	"strings"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	stunnerconfv1alpha1 "github.com/l7mp/stunner/pkg/apis/v1alpha1"

	stunnerv1alpha1 "github.com/l7mp/stunner-gateway-operator/api/v1alpha1"
	// "github.com/l7mp/stunner-gateway-operator/internal/operator"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
)

func stnrListenerName(gw *gatewayv1alpha2.Gateway, l *gatewayv1alpha2.Listener) string {
	return fmt.Sprintf("%s/%s", store.GetObjectKey(gw), string(l.Name))
}

func (r *Renderer) renderListener(gw *gatewayv1alpha2.Gateway, gwConf *stunnerv1alpha1.GatewayConfig, l *gatewayv1alpha2.Listener, rs []*gatewayv1alpha2.UDPRoute, ap *addrPort) (*stunnerconfv1alpha1.ListenerConfig, error) {
	r.log.V(4).Info("renderListener", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "public-addr",
		fmt.Sprintf("%#v", ap))

	proto := strings.ToUpper(string(l.Protocol))
	if proto != "UDP" && proto != "TCP" {
		return nil, fmt.Errorf("unsupported protocol: %s", proto)
	}

	minPort, maxPort := stunnerconfv1alpha1.DefaultMinRelayPort,
		stunnerconfv1alpha1.DefaultMaxRelayPort
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

	lc := stunnerconfv1alpha1.ListenerConfig{
		Name:         stnrListenerName(gw, l),
		Protocol:     string(l.Protocol),
		Addr:         "$STUNNER_ADDR", // Addr will be filled in from the pod environment
		Port:         int(l.Port),
		PublicAddr:   a,
		PublicPort:   p,
		MinRelayPort: minPort,
		MaxRelayPort: maxPort,
	}

	for _, r := range rs {
		lc.Routes = append(lc.Routes, store.GetObjectKey(r))
	}

	r.log.V(2).Info("renderListener ready", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "result",
		fmt.Sprintf("%#v", lc))

	return &lc, nil
}
