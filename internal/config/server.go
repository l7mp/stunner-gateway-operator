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

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	cdsserver "github.com/l7mp/stunner/pkg/config/server"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	"github.com/l7mp/stunner-gateway-operator/pkg/config"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

type Server struct {
	*cdsserver.Server
	configCh chan event.Event
	*ProgressTracker
	log logr.Logger
}

func NewCDSServer(addr string, logger logr.Logger) *Server {
	log := logger.WithName("cds-server")
	return &Server{
		Server:          cdsserver.New(addr, getNodeAddressPatcher(log), log),
		configCh:        make(chan event.Event, 10),
		ProgressTracker: NewProgressTracker(),
		log:             log,
	}
}

func (c *Server) Start(ctx context.Context) error {
	go func() {
		defer close(c.configCh)
		defer c.Close()

		for {
			select {
			case e := <-c.configCh:
				if e.GetType() != event.EventTypeUpdate {
					c.log.Info("Config discovery server received unknown event",
						"event", e.String())
					continue
				}

				c.ProgressUpdate(1)
				if err := c.ProcessUpdate(e.(*event.EventUpdate)); err != nil {
					c.log.Error(err, "Could not process config update event", "event",
						e.String())
				}
				c.ProgressUpdate(-1)

			case <-ctx.Done():
				return
			}
		}
	}()

	return c.Server.Start(ctx)
}

// GetConfigUpdateChannel returns the channel on which the config discovery server listenens to
// update resuests.
func (c *Server) GetConfigUpdateChannel() chan event.Event {
	return c.configCh
}

// ProcessUpdate processes new config events and updates the server with the current
// state-of-the-world.
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

	if err := c.Server.UpdateConfig(configs); err != nil {
		return err
	}

	c.Server.UpdateLicenseStatus(e.LicenseStatus)

	return nil
}

func getNodeAddressPatcher(log logr.Logger) cdsserver.ConfigNodePatcher {
	return func(conf *stnrv1.StunnerConfig, node string) *stnrv1.StunnerConfig {
		if conf == nil || len(conf.Listeners) == 0 {
			return conf
		}

		nodeAddr := ""
		nodeAddrType := corev1.NodeAddressType("")
		found, patched := false, false
		for i := range conf.Listeners {
			if conf.Listeners[i].Addr == config.NodeAddressPlaceholder {
				if !found {
					aType, addr, err := getNodeAddress(node)
					if err != nil {
						log.Error(err, "could not patch config with node address",
							"config-id", conf.Admin.Name, "node-name", node)
						nodeAddr = ""
					} else {
						nodeAddr = addr
						nodeAddrType = aType
						found = true
						log.V(4).Info("found node address for patching config", "config-id",
							conf.Admin.Name, "node-name", node, "address", nodeAddr,
							"type", nodeAddrType)
					}
				}
				if found && nodeAddr != "" {
					conf.Listeners[i].Addr = nodeAddr
					patched = true
				} else {
					conf.Listeners[i].Addr = opdefault.DefaultSTUNnerAddressEnvVarName // $STUNNER_ADDR
				}
			}
		}

		if patched {
			log.V(2).Info("patched config with node external IP/DNS address", "config-id",
				conf.Admin.Name, "node-name", node, "address", nodeAddr, "type", nodeAddrType)
		}

		return conf
	}
}

// getNodeAddress returns the node's external IP (if any)
// - if status.addresses contains an address of type ExternalIP, return it
// - if status.addresses contains an address of type NodeExternalDNS, try to resolve it and return the obtained IP
// - otherwise return an error
func getNodeAddress(node string) (corev1.NodeAddressType, string, error) {
	n := store.Nodes.GetObject(types.NamespacedName{Name: node})
	if n == nil {
		return corev1.NodeAddressType(""), "", errors.New("node not found")
	}
	aType, addr, ok := store.GetExternalAddress(n)
	if !ok || addr == "" {
		return corev1.NodeAddressType(""), "", errors.New("no ExternalIP or ExternalDNS address found in Node status")
	}
	// resolve: harmless if we already have an External IP, force IPv4 of possible
	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip4", addr)
	if err != nil {
		// try both IPv4 and IPv6
		var addrs []string
		addrs, err = net.LookupHost(addr)
		if err == nil && len(addrs) > 0 {
			ips = []net.IP{net.ParseIP(addrs[0])}
		}
	}
	if err != nil {
		return corev1.NodeAddressType(""), "", fmt.Errorf("could not resolve node address: %w", err)
	}
	if len(ips) == 0 {
		return corev1.NodeAddressType(""), "", errors.New("could not resolve node address: no IP address found")
	}
	return aType, ips[0].String(), nil
}
