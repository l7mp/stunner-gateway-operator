package config

// updater uploads client updates
import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	stnrapiv2 "github.com/l7mp/stunner/v2/pkg/apis/v2"
	cdsserver "github.com/l7mp/stunner/v2/pkg/config/server"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// Server is the config discovery server the dataplane pods watch. Starts only at the leader so CDS
// clients to standby operator pods are rejected.
type Server struct {
	*cdsserver.Server
	configCh chan event.Event
	*ProgressTracker
	log logr.Logger
}

func NewCDSServer(addr string, logger logr.Logger) *Server {
	log := logger.WithName("cds-server")
	return &Server{
		Server:          cdsserver.New(addr, nodeAddressPatcher(log), log),
		configCh:        make(chan event.Event, 10),
		ProgressTracker: NewProgressTracker(),
		log:             log,
	}
}

// Start opens the listener and serves config updates until the context ends.
func (c *Server) Start(ctx context.Context) error {
	if err := c.Server.Start(ctx); err != nil {
		return err
	}
	defer c.Close()

	for {
		select {
		case e := <-c.configCh:
			switch e.GetType() {
			case event.EventTypeUpdate:
				c.ProgressUpdate(1)
				if err := c.ProcessUpdate(e.(*event.EventUpdate)); err != nil {
					c.log.Error(err, "Could not process config update event", "event",
						e.String())
				}
				c.ProgressUpdate(-1)
			case event.EventTypeLicense:
				c.UpdateLicenseStatus(e.(*event.EventLicense).Status)
			default:
				c.log.Info("Config discovery server received unknown event",
					"event", e.String())
			}

		case <-ctx.Done():
			return nil
		}
	}
}

// GetConfigUpdateChannel returns the channel on which the config discovery server listenens to
// update resuests.
func (c *Server) GetConfigUpdateChannel() chan event.Event {
	return c.configCh
}

// ProcessUpdate processes new config events and updates the server with the current state of the
// world.
func (c *Server) ProcessUpdate(e *event.EventUpdate) error {
	c.log.Info("Processing config update event", "generation", e.Generation, "update",
		e.String())

	configs := []cdsserver.Config{}
	for _, conf := range e.ConfigQueue {
		id := conf.Admin.Name
		c.log.V(4).Info("Config update", "generation", e.Generation, "client", id, "config",
			conf.String())
		if namespace, name, ok := cdsserver.NamespacedName(id); ok {
			configs = append(configs, cdsserver.Config{
				Name:      name,
				Namespace: namespace,
				Config:    conf,
			})
		}
	}

	if err := c.UpdateConfig(configs); err != nil {
		return err
	}
	// a render follows every Node change: refresh what the patcher gives each pod
	c.Refresh()

	c.UpdateLicenseStatus(e.LicenseStatus)

	return nil
}

// nodeAddressPatcher relays each dataplane pod at the external address of its node: it replaces
// the node address marker (config.NodeAddressVar) among the cluster addresses. On a node without
// an external address it leaves the marker to the pod, whose own STUNNER_NODE_ADDR is its address.
func nodeAddressPatcher(log logr.Logger) cdsserver.Patcher {
	return func(conf *stnrapiv2.StunnerConfig, id string, labels map[string]string) *stnrapiv2.StunnerConfig {
		node, ok := labels[stnrapiv2.DefaultCDSNodeLabel]
		if !ok {
			return conf
		}
		_, addr, err := getNodeAddress(node)
		if err != nil {
			log.V(4).Info("no node address", "config-id", id, "node-name", node, "reason", err.Error())
			return conf
		}
		for i := range conf.Clusters {
			for j, a := range conf.Clusters[i].Addrs {
				if a == config.NodeAddressVar {
					conf.Clusters[i].Addrs[j] = addr
				}
			}
		}
		return conf
	}
}

func getNodeAddress(node string) (corev1.NodeAddressType, string, error) {
	n := store.Nodes.GetObject(types.NamespacedName{Name: node})
	if n == nil {
		return corev1.NodeAddressType(""), "", errors.New("node not found")
	}
	aType, addr, ok := store.GetExternalAddress(n)
	if !ok || addr == "" {
		return corev1.NodeAddressType(""), "", errors.New("no ExternalIP or ExternalDNS address found in Node status")
	}
	// resolve family-neutrally: a bare IP literal (v4 or v6) resolves to itself, a DNS name to its
	// A/AAAA records. This handles IPv6-only and dual-stack nodes; selecting a family across a
	// dual-stack result is a separate concern (see the IP-family selection work).
	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip", addr)
	if err != nil {
		return corev1.NodeAddressType(""), "", fmt.Errorf("could not resolve node address: %w", err)
	}
	if len(ips) == 0 {
		return corev1.NodeAddressType(""), "", errors.New("could not resolve node address: no IP address found")
	}
	return aType, ips[0].String(), nil
}
