// The MIT License (MIT)
//
// Copyright (c) 2015 xtaci
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

//go:build linux

package kcp

import (
	"net"
	"syscall"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type (
	platform struct {
		batchConn batchConn
	}

	// udpConn is an interface implemented by net.UDPConn.
	// It can be used for interface assertions to check if a net.Conn is a UDP connection.
	udpConn interface {
		SyscallConn() (syscall.RawConn, error)
		ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error)
	}

	// batchConn defines the interface used in batch IO
	batchConn interface {
		WriteBatch(ms []ipv4.Message, flags int) (int, error)
		ReadBatch(ms []ipv4.Message, flags int) (int, error)
	}
)

// newBatchConn creates a batchConn based on the IP version of the provided net.PacketConn.
func newBatchConn(conn net.PacketConn) batchConn {
	if _, ok := conn.(udpConn); !ok {
		return nil
	}

	// Resolve the local UDP address to determine IP version
	addr, err := net.ResolveUDPAddr("udp", conn.LocalAddr().String())
	if err != nil {
		return nil
	}

	// Determine if the connection is IPv4 or IPv6 based on the local address
	if addr.IP.To4() != nil {
		return ipv4.NewPacketConn(conn)
	}

	return ipv6.NewPacketConn(conn)
}

func (sess *UDPSession) initPlatform() {
	sess.platform.batchConn = newBatchConn(sess.conn)
}
