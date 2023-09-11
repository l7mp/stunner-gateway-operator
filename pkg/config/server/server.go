package server

// updater uploads client updates
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"

	corev1 "k8s.io/api/core/v1"

	cdsclient "github.com/l7mp/stunner/pkg/config/client"

	"github.com/l7mp/stunner-gateway-operator/internal/event"
	"github.com/l7mp/stunner-gateway-operator/internal/store"
	opdefault "github.com/l7mp/stunner-gateway-operator/pkg/config"
)

// ConfigWriteDeadline defines the deadline after which we consider a write event failed.
const ConfigWriteDeadline = 100 * time.Millisecond

type ConfigDiscoveryConfig struct {
	Addr   string
	Logger logr.Logger
}

type ConfigDiscoveryServer struct {
	ctx      context.Context
	addr     string
	configCh chan event.Event
	conns    map[string]*websocket.Conn
	lock     sync.RWMutex
	store    *store.ConfigMapStore
	log      logr.Logger
}

func NewConfigDiscoveryServer(cfg ConfigDiscoveryConfig) *ConfigDiscoveryServer {
	return &ConfigDiscoveryServer{
		configCh: make(chan event.Event, 10),
		addr:     cfg.Addr,
		conns:    make(map[string]*websocket.Conn),
		store:    store.NewConfigMapStore(),
		log:      cfg.Logger.WithName("cds-server"),
	}
}

func (c *ConfigDiscoveryServer) Start(ctx context.Context) error {
	c.ctx = ctx

	// init server
	s := &http.Server{Addr: c.addr}

	// config watcher API
	http.HandleFunc(opdefault.DefaultConfigDiscoveryEndpoint+"/watch",
		func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{
				ReadBufferSize:  1024,
				WriteBufferSize: 1024,
			}

			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				c.log.Error(err, "could not upgrade HTTP connection", "client",
					r.RemoteAddr)
			}

			c.HandleConn(ctx, conn, r)
		})

	// config API
	http.HandleFunc(opdefault.DefaultConfigDiscoveryEndpoint,
		func(w http.ResponseWriter, r *http.Request) {
			c.HandleReq(w, r)
		})

	// serve
	go func() {
		if err := s.ListenAndServe(); err != nil {
			c.log.Info("closing config discovery server", "event", err.Error())
			return
		}
	}()

	c.log.Info("config discovery server running", "address", c.addr,
		"config-path", opdefault.DefaultConfigDiscoveryEndpoint)

	// listen to config update events and cancel requests
	go func() {
		defer close(c.configCh)
		defer s.Close()

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

	return nil
}

// GetConfigUpdateChannel returns the channel on which the config discovery server listenens to
// update resuests.
func (c *ConfigDiscoveryServer) GetConfigUpdateChannel() chan event.Event {
	return c.configCh
}

// HandleReq handles config requests.
func (c *ConfigDiscoveryServer) HandleReq(w http.ResponseWriter, r *http.Request) {
	id, err := c.getClientId(r)
	if err != nil {
		c.log.V(2).Error(err, "invalid client id")
		http.Error(w, "Invalid client id", http.StatusBadRequest)
		return
	}

	c.log.V(1).Info("received new client request", "id", id, "config-store", c.store.String())

	namespacedName := store.GetNameFromKey(id)
	cm := c.store.GetObject(namespacedName)

	if cm == nil {
		c.log.V(2).Info("no config", "client", id)
		http.Error(w, "No config", http.StatusBadRequest)
		return
	}

	// obtain config
	conf, ok := cm.Data[opdefault.DefaultStunnerdConfigfileName]
	if !ok {
		c.log.V(2).Info("error: no stunnerd config found in config-map", "id", id)
		http.Error(w, "No stunnerd config found in config-map", http.StatusInternalServerError)
		return
	}

	c.log.V(4).Info("sending config to client", "client", id, "config", store.DumpObject(cm))

	// and send it along!
	if _, err := w.Write([]byte(conf)); err != nil {
		c.log.Error(err, "could not write config", "id", id)
		http.Error(w, "Could not write config", http.StatusInternalServerError)
		return
	}
}

// HandleConn handles a new client WebSocket connection.
func (c *ConfigDiscoveryServer) HandleConn(ctx context.Context, conn *websocket.Conn, req *http.Request) {
	id, err := c.getClientId(req)
	if err != nil {
		c.log.V(2).Error(err, "invalid client connection request", "client", req.RemoteAddr)
		return
	}

	c.log.V(1).Info("received new client connection", "client", conn.RemoteAddr().String(), "id", id,
		"config-store", c.store.String())

	c.lock.RLock()
	client, ok := c.conns[id]
	c.lock.RUnlock()

	if ok {
		c.log.V(1).Info("client connection already exists, dropping old connection",
			"client", conn.RemoteAddr().String(), "id", id)
		client.Close()
	}

	c.lock.Lock()
	c.conns[id] = conn
	c.lock.Unlock()

	// a dummy reader that drops everything it receives: this must be there for the WebSocket
	// server to call our pong-handler: conn.Close() will kill this goroutine
	go func() {
		for {
			// drop anything we receive
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	conn.SetPingHandler(func(string) error {
		return conn.WriteMessage(websocket.PongMessage, []byte("keepalive"))
	})

	// send initial config
	if err = c.sendConfig(id); err != nil {
		c.log.Error(err, "cannot send initial configuration", "client",
			conn.RemoteAddr().String(), "id", id)
	}

	// wait until client closes the connection or the server is cancelled
	// note: it is the responsiility of the client to check connection liveliness and reconnect
	// if something goes wrong
	select {
	case <-ctx.Done():
	case <-req.Context().Done():
	}

	c.log.V(1).Info("client connection closed", "client", conn.LocalAddr().String(), "id", id)

	conn.Close()
}

// ProcessUpdate processes new config events. If first takes all locally stored configmaps
// that do not appear in the update and wipes to config from the corresponding clients, then stores
// the new configmaps, checks is anything has changed, and if yes, sends an update.
func (c *ConfigDiscoveryServer) ProcessUpdate(e *event.EventUpdate) error {
	c.log.Info("processing config update event", "generation", e.Generation,
		"update", e.String())

	q := e.UpsertQueue

	// wipe all old configs that have disappeared
	for _, cm := range c.store.GetAll() {
		nsName := store.GetNamespacedName(cm)
		o := q.ConfigMaps.Get(nsName)
		if o == nil {
			c.log.V(4).Info("removing config", "generation", e.Generation,
				"client", nsName.String())

			// config has disappeared: remove from local store
			c.store.Remove(nsName)
			// this will send an empty config to the client, if online
			id := nsName.String()
			if err := c.sendConfig(id); err != nil {
				c.log.V(1).Info("cannot send config (client has gone?)", "client", id,
					"config-map", store.DumpObject(cm), "error", err)
				continue
			}
		}

	}

	// store and send each new configmap if something has changed
	for _, cm := range q.ConfigMaps.GetAll() {
		nsName := store.GetNamespacedName(cm)
		id := nsName.String()
		o := c.store.GetObject(nsName)
		if o != nil && cmEqual(cm, o) {
			// configmap has not changed
			c.log.V(4).Info("config unchanged", "generation", e.Generation,
				"client", nsName.String())
			continue
		}

		// new config!
		c.log.V(4).Info("new config", "generation", e.Generation,
			"client", nsName.String())
		c.store.Upsert(cm)
		if err := c.sendConfig(id); err != nil {
			c.log.V(1).Info("cannot send config (client has gone?)", "client", id,
				"config-map", store.DumpObject(cm), "error", err)
			continue
		}
	}

	// configmaps are never deleted so the delete queue is always empty

	return nil
}

// sendConfig sends a config to a client. If no config is stored for the client, send an empty
// config (this is also used for deleting configs from clients).
func (c *ConfigDiscoveryServer) sendConfig(id string) error {
	// obtain client connection
	c.lock.RLock()
	conn, ok := c.conns[id]
	c.lock.RUnlock()

	if !ok {
		return nil
	}

	// obtain fresh config
	var conf []byte
	nsName := store.GetNameFromKey(id)
	cm := c.store.GetObject(nsName)
	if cm == nil {
		z := cdsclient.ZeroConfig(id)
		json, err := json.Marshal(z)
		if err != nil {
			return err
		}

		conf = json

		c.log.V(4).Info("sending empty config to client", "client", id, "config", string(conf))
	} else {

		// obtain config
		z, ok := cm.Data[opdefault.DefaultStunnerdConfigfileName]
		if !ok {
			return fmt.Errorf("no stunnerd config found in config-map")
		}

		c.log.V(4).Info("sending config to client", "client", id, "config", z)

		conf = []byte(z)
	}

	// and send it along
	if err := conn.WriteMessage(websocket.TextMessage, conf); err != nil {
		c.closeConn(conn, id)

		return fmt.Errorf("could not send config: %w", err)
	}

	return nil
}

func (c *ConfigDiscoveryServer) closeConn(conn *websocket.Conn, id string) {
	c.log.V(1).Info("closing connection", "client", conn.RemoteAddr().String(), "id", id)
	conn.WriteMessage(websocket.CloseMessage, []byte{}) //nolint:errcheck
	c.lock.Lock()
	delete(c.conns, id)
	c.lock.Unlock()
	conn.Close()
}

func (c *ConfigDiscoveryServer) getClientId(req *http.Request) (string, error) {
	u := req.URL
	if u == nil {
		return "", errors.New("cannot obtain request URL")
	}

	id := u.Query().Get("id")
	if id == "" {
		return "", fmt.Errorf("client id not set in request query %q", u.String())
	}

	// valid id?
	ss := strings.Split(id, "/")
	if len(ss) != 2 {
		return "", fmt.Errorf("malformed client id %q", id)
	}

	return id, nil
}

func cmEqual(cm1, cm2 *corev1.ConfigMap) bool {
	if cm1 == nil || cm2 == nil {
		return false
	}

	conf1, ok := cm1.Data[opdefault.DefaultStunnerdConfigfileName]
	if !ok {
		return false
	}

	s1Conf, err := cdsclient.ParseConfig([]byte(conf1))
	if err != nil {
		return false
	}

	conf2, ok := cm2.Data[opdefault.DefaultStunnerdConfigfileName]
	if !ok {
		return false
	}

	s2Conf, err := cdsclient.ParseConfig([]byte(conf2))
	if err != nil {
		return false
	}

	return s1Conf.DeepEqual(s2Conf)

}
