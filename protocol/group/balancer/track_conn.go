package balancer

import (
	"net"
	"sync"

	"github.com/sagernet/sing-box/adapter"
	N "github.com/sagernet/sing/common/network"
)

type trackedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *trackedConn) Close() error {
	c.once.Do(c.release)
	return c.Conn.Close()
}

func (c *trackedConn) Upstream() any { return c.Conn }

type trackedPacketConn struct {
	net.PacketConn
	once    sync.Once
	release func()
}

func (c *trackedPacketConn) Close() error {
	c.once.Do(c.release)
	return c.PacketConn.Close()
}

func (c *trackedPacketConn) Upstream() any { return c.PacketConn }

func wrapConnLifecycle(conn net.Conn, life connectionLifecycle, out adapter.Outbound) net.Conn {
	if life == nil || out == nil || conn == nil {
		return conn
	}
	life.Opened(out)
	return &trackedConn{Conn: conn, release: func() { life.Closed(out) }}
}

func wrapPacketLifecycle(conn net.PacketConn, life connectionLifecycle, out adapter.Outbound) net.PacketConn {
	if life == nil || out == nil || conn == nil {
		return conn
	}
	life.Opened(out)
	return &trackedPacketConn{PacketConn: conn, release: func() { life.Closed(out) }}
}

func wrapOnCloseLifecycle(onClose N.CloseHandlerFunc, life connectionLifecycle, out adapter.Outbound) N.CloseHandlerFunc {
	if life == nil || out == nil {
		return onClose
	}
	life.Opened(out)
	return func(err error) {
		life.Closed(out)
		if onClose != nil {
			onClose(err)
		}
	}
}
