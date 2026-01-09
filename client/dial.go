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

package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"github.com/pkg/errors"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/kcptun/std"
	"github.com/xtaci/tcpraw"
)

var (
	multiPort           *std.MultiPort
	multiPortParseError error
	multiPortOnce       sync.Once
)

// dial connects to the remote address
func dial(config *Config, block kcp.BlockCrypt) (*kcp.UDPSession, error) {
	// initialize multiPort once
	multiPortOnce.Do(func() {
		multiPort, multiPortParseError = std.ParseMultiPort(config.RemoteAddr)
	})

	// return error if multiPort parsing failed
	if multiPortParseError != nil {
		return nil, multiPortParseError
	}

	// generate a random port
	var randport uint64
	err := binary.Read(rand.Reader, binary.LittleEndian, &randport)
	if err != nil {
		return nil, err
	}

	remoteAddr := fmt.Sprintf("%v:%v", multiPort.Host, uint64(multiPort.MinPort)+randport%uint64(multiPort.MaxPort-multiPort.MinPort+1))

	// emulate TCP connection
	if config.TCP {
		conn, err := tcpraw.Dial("tcp", remoteAddr)
		if err != nil {
			return nil, errors.Wrap(err, "tcpraw.Dial()")
		}

		udpaddr, err := net.ResolveUDPAddr("udp", remoteAddr)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		var convid uint32
		binary.Read(rand.Reader, binary.LittleEndian, &convid)
		return kcp.NewConn4(convid, udpaddr, block, config.DataShard, config.ParityShard, true, conn)
	}

	// default UDP connection
	return kcp.DialWithOptions(remoteAddr, block, config.DataShard, config.ParityShard)
}
