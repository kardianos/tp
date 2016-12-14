package internal

import (
	"net"
	"time"

	"github.com/dchest/spipe"
)

type listener struct {
	net.Listener
	key       []byte
	keepalive time.Duration
}

// Listen announces on the local network address laddr, which
// will accept spipe client connections with the given shared
// secret key.
func Listen(key []byte, network, laddr string, keepalive time.Duration) (net.Listener, error) {
	nl, err := net.Listen(network, laddr)
	if err != nil {
		return nil, err
	}
	return &listener{
		Listener:  nl,
		key:       key,
		keepalive: keepalive,
	}, nil
}

// Accept waits for and returns the next connection to the listener.
// The returned connection c is a *spipe.Conn.
func (l *listener) Accept() (c net.Conn, err error) {
	nc, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if l.keepalive > 0 {
		defer func() {
			if err != nil {
				nc.Close()
			}
		}()
		tcp := nc.(*net.TCPConn)
		err = tcp.SetKeepAlivePeriod(l.keepalive)
		if err != nil {
			return nil, err
		}
		err = tcp.SetKeepAlive(true)
		if err != nil {
			return nil, err
		}
	}
	return spipe.Server(l.key, nc), nil
}
