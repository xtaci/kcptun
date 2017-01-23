// The MIT License (MIT)
//
// Copyright (c) 2016 xtaci
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
	"io"
	"sync"
	"time"
)

const (
	bufSize = 4096
)

// bufPool is a pool of byte slices used for io.Copy operations.
// Reusing buffers reduces GC pressure under high concurrency.
var bufPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, bufSize)
		return &buf
	},
}

// Memory optimized io.Copy function specified for this library
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}

	// fallback to standard io.CopyBuffer with pooled buffer
	bufPtr := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufPtr)
	return io.CopyBuffer(dst, src, *bufPtr)
}

// closeWriter is an interface for connections that support half-close (CloseWrite).
// Both net.TCPConn and smux.Stream implement this interface.
type closeWriter interface {
	CloseWrite() error
}

// Pipe create a general bidirectional pipe between two streams
// It uses CloseWrite() for half-close to allow graceful shutdown:
// when one direction finishes, it signals the peer that no more data will be sent,
// while still allowing data to be received from the other direction.
func Pipe(alice, bob io.ReadWriteCloser, closeWait int) (errA, errB error) {
	var wg sync.WaitGroup
	wg.Add(2)

	streamCopy := func(dst io.ReadWriteCloser, src io.ReadWriteCloser, err *error) {
		defer wg.Done()

		// write error directly to the *pointer
		_, *err = Copy(dst, src)

		if closeWait > 0 {
			time.Sleep(time.Duration(closeWait) * time.Second)
		}

		// half-close: signal the peer that we are done writing
		// use CloseWrite if available, otherwise fall back to Close
		if cw, ok := dst.(closeWriter); ok {
			cw.CloseWrite()
		} else {
			dst.Close()
		}
	}

	// start bidirectional stream copying
	// alice -> bob: read from alice, write to bob
	// bob -> alice: read from bob, write to alice
	go streamCopy(bob, alice, &errA) // alice->bob direction
	go streamCopy(alice, bob, &errB) // bob->alice direction

	// wait for both direction to close
	wg.Wait()

	// fully close both connections after both directions are done
	alice.Close()
	bob.Close()

	return
}
