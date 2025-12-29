// The MIT License (MIT)
//
// # Copyright (c) 2015 xtaci
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
	"errors"
	"sync"
)

// A system-wide packet buffer shared among sending, receiving and FEC
// to mitigate high-frequency memory allocation of packets.
var defaultBufferPool = newBufferPool(mtuLimit)

type bufferPool struct {
	xmitBuf sync.Pool
}

// newBufferPool creates a new buffer pool with buffers of the given size.
func newBufferPool(size int) *bufferPool {
	return &bufferPool{
		xmitBuf: sync.Pool{
			New: func() any {
				return make([]byte, size)
			},
		},
	}
}

// Get retrieves a buffer from the pool.
func (bp *bufferPool) Get() []byte {
	return bp.xmitBuf.Get().([]byte)
}

// Put returns a buffer to the pool.
func (bp *bufferPool) Put(buf []byte) error {
	// Only put back buffers of the correct size.
	if cap(buf) != mtuLimit {
		return errors.New("buffer size mismatch")
	}
	bp.xmitBuf.Put(buf)
	return nil
}
