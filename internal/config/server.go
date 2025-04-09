package config

// updater uploads client updates
import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/go-logr/logr"
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

		var nodeAddr *string
		for i := range conf.Listeners {
			if conf.Listeners[i].Addr == config.NodeAddressPlaceholder {
				if nodeAddr == nil {
					addr, err := getNodeAddress(node)
					if err != nil {
						log.Error(err, "could not patch config with node address",
							"node-name", node, "config-id", conf.Admin.Name)
						addr = ""
					}
					nodeAddr = &addr
				}
				if *nodeAddr != "" {
					conf.Listeners[i].Addr = *nodeAddr
				} else {
					conf.Listeners[i].Addr = opdefault.DefaultSTUNnerAddressEnvVarName // $STUNNER_ADDR
				}
			}

		}

		return conf
	}
}

// getNodeAddress returns the node's external IP (if any)
// - if status.addresses contains an address of type ExternalIP, return it
// - if status.addresses contains an address of type NodeExternalDNS, try to resolve it and return the obtained IP
// - otherwise return an error
func getNodeAddress(node string) (string, error) {
	n := store.Nodes.GetObject(types.NamespacedName{Name: node})
	if n == nil {
		return "", errors.New("node not found")
	}
	addr := store.GetExternalAddress(n)
	if addr == "" {
		return "", errors.New("node ExternalIP or ExternalDNS address found in Node status")
	}
	ips, err := net.LookupHost(addr)
	if err != nil || len(ips) == 0 {
		return "", fmt.Errorf("could not resolve node address")
	}
	return ips[0], nil
}
