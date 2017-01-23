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

package kcp

import (
	"net"

	"github.com/pkg/errors"
)

func sameUDPAddr(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Port != b.Port || a.Zone != b.Zone {
		return false
	}
	return a.IP.Equal(b.IP)
}

// defaultReadLoop is the standard procedure for reading from a connection
func (s *UDPSession) defaultReadLoop() {
	buf := make([]byte, maxPacketSize)

	var src *net.UDPAddr
	var srcStr string
	if s.remote != nil {
		if udp, ok := s.remote.(*net.UDPAddr); ok {
			src = udp
		} else {
			srcStr = s.remote.String()
		}
	}
	for {
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			s.notifyReadError(errors.WithStack(err))
			return
		}

		if s.isClosed() {
			return
		}

		// Ensure the packet comes from the same source.
		if src == nil && srcStr == "" { // set source address if nil
			if udp, ok := addr.(*net.UDPAddr); ok {
				src = udp
			} else {
				srcStr = addr.String()
			}
		} else if src != nil {
			udp, ok := addr.(*net.UDPAddr)
			if !ok || !sameUDPAddr(src, udp) {
				DefaultSnmp.increaseInErrs()
				continue
			}
		} else if addr.String() != srcStr {
			DefaultSnmp.increaseInErrs()
			continue
		}

		s.packetInput(buf[:n])
	}
}

// defaultReadLoop is the standard procedure for reading and accepting connections on a listener
func (l *Listener) defaultMonitor() {
	buf := make([]byte, maxPacketSize)
	for {
		n, from, err := l.conn.ReadFrom(buf)
		if err != nil {
			l.notifyReadError(errors.WithStack(err))
			return
		}

		l.packetInput(buf[:n], from)
	}
}

// PacketPusher is a net.PacketConn that can hand each received packet to a
// callback instead of being read. A client session or Listener on such a
// conn runs no read goroutine and processes each packet inside the callback,
// OOB handlers included, so a slow handler delays every conn the pusher
// serves.
//
// The pusher must not call fn concurrently, just as one goroutine reads a
// conn. fn may modify b but must not keep it after returning. A non-nil err
// ends reads. Closing a client session, or a Listener once its sessions have
// closed, sets a nil fn, which must remove the receiver.
type PacketPusher interface {
	SetPacketReceiver(fn func(b []byte, addr net.Addr, err error))
}

// pushedPacket is the PacketPusher callback of a client session.
func (s *UDPSession) pushedPacket(b []byte, addr net.Addr, err error) {
	if err != nil {
		s.notifyReadError(errors.WithStack(err))
		return
	}
	if s.isClosed() {
		return
	}
	// Without a remote address, pin the peer to the first packet as
	// defaultReadLoop does.
	src := s.remote
	if src == nil {
		if s.pushedFrom == nil {
			s.pushedFrom = addr
		}
		src = s.pushedFrom
	}
	if udp, ok := src.(*net.UDPAddr); ok {
		if a, ok := addr.(*net.UDPAddr); !ok || !sameUDPAddr(udp, a) {
			DefaultSnmp.increaseInErrs()
			return
		}
	} else if addr.String() != src.String() {
		DefaultSnmp.increaseInErrs()
		return
	}
	s.packetInput(b)
}

// pushedPacket is the PacketPusher callback of a Listener.
func (l *Listener) pushedPacket(b []byte, addr net.Addr, err error) {
	if err != nil {
		l.notifyReadError(errors.WithStack(err))
		return
	}
	l.packetInput(b, addr)
}
