package listener

import (
	"net"
	"os"
)

// AbortiveCloseWhenRoot makes connections accepted on a local control port
// (gRPC API, Clash API) close with RST instead of FIN while the core runs as
// root.
//
// Why: when a root core exits, its side of every open control connection
// would sit in TIME_WAIT on the control port for 2*MSL (30s on macOS).
// BSD/macOS refuses to let a different, unprivileged user bind a port held by
// another user's socket, even with SO_REUSEADDR, so the unprivileged core
// that replaces the root one (TUN switched off) fails with "address already in
// use". SO_LINGER=0 skips TIME_WAIT. Only local, app-owned clients use these
// ports, so dropping unsent bytes on close is acceptable there.
//
// Outside root (or on platforms without geteuid) the listener is returned
// unchanged.
func AbortiveCloseWhenRoot(listener net.Listener) net.Listener {
	if os.Geteuid() != 0 {
		return listener
	}
	return &abortiveListener{Listener: listener}
}

type abortiveListener struct {
	net.Listener
}

func (l *abortiveListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tcpConn, isTCP := conn.(*net.TCPConn); isTCP {
		_ = tcpConn.SetLinger(0)
	}
	return conn, nil
}
