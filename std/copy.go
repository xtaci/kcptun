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
	"io"
	"sync"
)

const bufSize = 4096

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

	// fallback to standard io.CopyBuffer
	buf := make([]byte, bufSize)
	return io.CopyBuffer(dst, src, buf)
}

// Pipe create a general bidirectional pipe between two streams
func Pipe(alice, bob io.ReadWriteCloser) (errA, errB error) {
	var closed sync.Once

	var wg sync.WaitGroup
	wg.Add(2)

	streamCopy := func(dst io.Writer, src io.ReadCloser, err *error) {
		// write error directly to the *pointer
		_, *err = Copy(dst, src)
		wg.Done()

		// close only once
		closed.Do(func() {
			alice.Close()
			bob.Close()
		})
	}

	// start bidirectional stream copying
	go streamCopy(alice, bob, &errA)
	go streamCopy(bob, alice, &errB)

	// wait for both direction to close
	wg.Wait()

	return
}
