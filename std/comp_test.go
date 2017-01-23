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
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
)

func TestCompStreamRoundTrip(t *testing.T) {
	left, right := net.Pipe()
	compWriter := NewCompStream(left)
	compReader := NewCompStream(right)
	t.Cleanup(func() {
		compWriter.Close()
		compReader.Close()
	})

	payload := bytes.Repeat([]byte("compressed payload"), 64)
	readErr := make(chan error, 1)

	go func() {
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(compReader, buf); err != nil {
			readErr <- fmt.Errorf("read compressed data: %w", err)
			return
		}
		if !bytes.Equal(buf, payload) {
			sample := buf
			if len(sample) > 64 {
				sample = sample[:64]
			}
			readErr <- fmt.Errorf("unexpected payload prefix: %x", sample)
			return
		}
		readErr <- nil
	}()

	writeBuf := append([]byte(nil), payload...)
	if n, err := compWriter.Write(writeBuf); err != nil {
		t.Fatalf("compWriter.Write error: %v", err)
	} else if n != len(writeBuf) {
		t.Fatalf("write returned %d, want %d", n, len(writeBuf))
	}

	if err := compWriter.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	if err := <-readErr; err != nil {
		t.Fatalf("reader error: %v", err)
	}
}
