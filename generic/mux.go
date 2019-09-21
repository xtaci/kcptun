package generic

import (
	"io"
	"net"
)

type Mux interface {
	Open() (io.ReadWriteCloser, error)
	Accept() (io.ReadWriteCloser, error)
	IsClosed() bool
	NumStreams() int
	RemoteAddr() net.Addr
	Close() error
}

type Stream interface {
	io.ReadWriteCloser
	ID() int
	RemoteAddr() net.Addr
}
