package generic

import (
	"net"
	"time"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
)

type CompStream struct {
	conn net.Conn
	w    *snappy.Writer
	r    *snappy.Reader
}

func (c *CompStream) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}

func (c *CompStream) Write(p []byte) (n int, err error) {
	if _, err := c.w.Write(p); err != nil {
		return 0, errors.WithStack(err)
	}

	if err := c.w.Flush(); err != nil {
		return 0, errors.WithStack(err)
	}
	return len(p), err
}

func (c *CompStream) Close() error {
	return c.conn.Close()
}

func (c *CompStream) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *CompStream) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *CompStream) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *CompStream) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *CompStream) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

func NewCompStream(conn net.Conn) *CompStream {
	c := new(CompStream)
	c.conn = conn
	c.w = snappy.NewBufferedWriter(conn)
	c.r = snappy.NewReader(conn)
	return c
}
