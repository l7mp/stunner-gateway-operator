package config

// updater uploads client updates
import (
	"context"

	"github.com/go-logr/logr"

	cdsserver "github.com/l7mp/stunner/pkg/config/server"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

type Server struct {
	*cdsserver.Server
	configCh chan event.Event
	*ProgressTracker
	log logr.Logger
}

func NewCDSServer(addr string, logger logr.Logger) *Server {
	return newCDSServer(addr, nil, logger)
}

func newCDSServer(addr string, patcher cdsserver.ConfigNodePatcher, logger logr.Logger) *Server {
	log := logger.WithName("cds-server")
	return &Server{
		Server:          cdsserver.New(addr, patcher, log),
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
