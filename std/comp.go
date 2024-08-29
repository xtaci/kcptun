// The MIT License (MIT)
//
// # Copyright (c) 2016 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package std

import (
	"net"
	"time"

	"github.com/golang/snappy"
	"github.com/pkg/errors"
)

// CompStream is a net.Conn wrapper that compresses data using snappy
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

// NewCompStream creates a new stream that compresses data using snappy
func NewCompStream(conn net.Conn) *CompStream {
	c := new(CompStream)
	c.conn = conn
	c.w = snappy.NewBufferedWriter(conn)
	c.r = snappy.NewReader(conn)
	return c
}
