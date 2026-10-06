package server

import (
	"crypto/tls"
	"net"
	"sync"
)

// SniffTLS serves plain HTTP and TLS on one port: a connection whose first byte is a
// TLS handshake record (0x16) is wrapped in TLS, anything else stays plain. The choice
// is made on the first read, so a slow client never holds up Accept.
func SniffTLS(ln net.Listener, cfg *tls.Config) net.Listener {
	return &sniffListener{Listener: ln, cfg: cfg}
}

type sniffListener struct {
	net.Listener
	cfg *tls.Config
}

func (l *sniffListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &sniffConn{Conn: c, cfg: l.cfg}, nil
}

type sniffConn struct {
	net.Conn // the raw connection: deadlines and addresses go straight to it
	cfg      *tls.Config

	once  sync.Once
	inner net.Conn // TLS or the raw connection with the first byte put back
	err   error
}

func (c *sniffConn) decide() {
	c.once.Do(func() {
		var first [1]byte
		n, err := c.Conn.Read(first[:])
		if n == 0 {
			c.err = err
			return
		}
		raw := &prefixConn{Conn: c.Conn, prefix: first[:n]}
		if first[0] == 0x16 {
			c.inner = tls.Server(raw, c.cfg)
		} else {
			c.inner = raw
		}
	})
}

func (c *sniffConn) Read(p []byte) (int, error) {
	c.decide()
	if c.inner == nil {
		return 0, c.err
	}
	return c.inner.Read(p)
}

func (c *sniffConn) Write(p []byte) (int, error) {
	c.decide()
	if c.inner == nil {
		return 0, c.err
	}
	return c.inner.Write(p)
}

func (c *sniffConn) Close() error {
	if c.inner != nil {
		return c.inner.Close()
	}
	return c.Conn.Close()
}

// prefixConn returns the bytes already read before reading on.
type prefixConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}
