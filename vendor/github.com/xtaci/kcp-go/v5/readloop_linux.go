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
	"sync/atomic"

	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
)

const (
	batchSize = 256
)

// readLoop is the optimized version of readLoop for linux utilizing recvmmsg syscall
func (s *UDPSession) readLoop() {
	// default version
	if s.platform.batchConn == nil {
		s.defaultReadLoop()
		return
	}

	// x/net version
	var src *net.UDPAddr
	var srcStr string
	if s.remote != nil {
		if udp, ok := s.remote.(*net.UDPAddr); ok {
			src = udp
		} else {
			srcStr = s.remote.String()
		}
	}
	msgs := make([]ipv4.Message, batchSize)
	for k := range msgs {
		msgs[k].Buffers = [][]byte{make([]byte, mtuLimit)}
	}

	for {
		count, err := s.platform.batchConn.ReadBatch(msgs, 0)
		if err != nil {
			s.notifyReadError(errors.WithStack(err))
			return
		}

		if s.isClosed() {
			return
		}

		for i := range count {
			msg := &msgs[i]

			// make sure the packet is from the same source
			if src == nil && srcStr == "" { // set source address if nil
				if udp, ok := msg.Addr.(*net.UDPAddr); ok {
					src = udp
				} else {
					srcStr = msg.Addr.String()
				}
			} else if src != nil {
				udp, ok := msg.Addr.(*net.UDPAddr)
				if !ok || !sameUDPAddr(src, udp) {
					atomic.AddUint64(&DefaultSnmp.InErrs, 1)
					continue
				}
			} else if msg.Addr.String() != srcStr {
				atomic.AddUint64(&DefaultSnmp.InErrs, 1)
				continue
			}

			// source and size has validated
			s.packetInput(msg.Buffers[0][:msg.N])
		}
	}
}

// monitor is the optimized version of monitor for linux utilizing recvmmsg syscall
func (l *Listener) monitor() {
	batchConn := newBatchConn(l.conn)

	// default version
	if batchConn == nil {
		l.defaultMonitor()
		return
	}

	// x/net version
	msgs := make([]ipv4.Message, batchSize)
	for k := range msgs {
		msgs[k].Buffers = [][]byte{make([]byte, mtuLimit)}
	}

	for {
		count, err := batchConn.ReadBatch(msgs, 0)
		if err != nil {
			l.notifyReadError(errors.WithStack(err))
			return
		}

		for i := range count {
			msg := &msgs[i]
			l.packetInput(msg.Buffers[0][:msg.N], msg.Addr)
		}
	}
}
