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

func (r *Renderer) renderListener(gw *gatewayv1alpha2.Gateway, gwConf *stunnerv1alpha1.GatewayConfig, l *gatewayv1alpha2.Listener, rs []*gatewayv1alpha2.UDPRoute, addr gatewayv1alpha2.GatewayAddress) (*stunnerconfv1alpha1.ListenerConfig, error) {
	r.log.V(4).Info("renderListener", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "public-addr",
		addr)

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

	lc := stunnerconfv1alpha1.ListenerConfig{
		Name:         string(l.Name),
		Protocol:     string(l.Protocol),
		Addr:         "$STUNNER_ADDR", // Addr will be filled in from the pod environment
		PublicAddr:   addr.Value,
		Port:         int(l.Port),
		MinRelayPort: minPort,
		MaxRelayPort: maxPort,
	}

	for _, r := range rs {
		lc.Routes = append(lc.Routes, r.Name)
	}

	r.log.V(2).Info("renderListener ready", "gateway", store.GetObjectKey(gw), "gateway-config",
		store.GetObjectKey(gwConf), "listener", l.Name, "route number", len(rs), "result",
		fmt.Sprintf("%#v", lc))

	return &lc, nil
}
