package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	readWait   = 30 * time.Second // phone pings every 10s
	serverPing = 15 * time.Second
)

// client is one connected phone. All writes go through the send channel so that a
// single goroutine owns the connection's writer.
type client struct {
	deviceID string
	conn     *websocket.Conn
	send     chan []byte
	once     sync.Once
	done     chan struct{}
}

func newClient(deviceID string, conn *websocket.Conn) *client {
	return &client{deviceID: deviceID, conn: conn, send: make(chan []byte, 16), done: make(chan struct{})}
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.done)
		c.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		c.conn.Close()
	})
}

// queue drops the client if it cannot keep up instead of blocking the broadcaster.
func (c *client) queue(msg []byte) {
	select {
	case c.send <- msg:
	case <-c.done:
	default:
		c.close()
	}
}

func (c *client) writeLoop() {
	ping := time.NewTicker(serverPing)
	defer ping.Stop()
	defer c.close()
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ping.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}

type hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

func (h *hub) add(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients == nil {
		h.clients = map[*client]struct{}{}
	}
	h.clients[c] = struct{}{}
}

func (h *hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
}

func (h *hub) broadcast(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.queue(msg)
	}
}

// kick disconnects every session of a device (used when it is unpaired).
func (h *hub) kick(deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.deviceID == deviceID {
			c.close()
		}
	}
}

func (h *hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		c.close()
	}
}

func (h *hub) online() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[string]bool{}
	for c := range h.clients {
		out[c.deviceID] = true
	}
	return out
}
