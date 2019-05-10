package generic

import (
	"net"

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

func NewCompStream(conn net.Conn) *CompStream {
	c := new(CompStream)
	c.conn = conn
	c.w = snappy.NewBufferedWriter(conn)
	c.r = snappy.NewReader(conn)
	return c
}
