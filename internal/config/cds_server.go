package config

// updater uploads client updates
import (
	"context"

	"github.com/go-logr/logr"

	stnrv1 "github.com/l7mp/stunner/pkg/apis/v1"
	cdsserverbase "github.com/l7mp/stunner/pkg/config/server"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
)

type Server struct {
	*cdsserverbase.Server
	configCh chan event.Event
	log      logr.Logger
}

func NewCDSServer(addr string, logger logr.Logger) *Server {
	log := logger.WithName("cds-server")
	return &Server{
		Server:   cdsserverbase.New(addr, log),
		configCh: make(chan event.Event, 10),
		log:      log,
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
					c.log.Info("config discovery server received unknown event",
						"event", e.String())
					continue
				}

				if err := c.ProcessUpdate(e.(*event.EventUpdate)); err != nil {
					c.log.Error(err, "could not process config update event", "event",
						e.String())
				}

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
	c.log.Info("processing config update event", "generation", e.Generation, "update",
		e.String())

	configs := []cdsserverbase.Config{}

	for _, conf := range e.ConfigQueue {
		id := conf.Admin.Name
		c.log.V(4).Info("new config", "generation", e.Generation, "client", id, "config",
			conf.String())

		// make sure we do not share pointers
		nc := stnrv1.StunnerConfig{}
		conf.DeepCopyInto(&nc)
		configs = append(configs, cdsserverbase.Config{
			Id:     id,
			Config: &nc,
		})
	}

	if err := c.Server.UpdateConfig(configs); err != nil {
		return err
	}

	return nil
}
