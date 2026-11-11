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

	"github.com/xtaci/qpp"
)

func TestQPPPortRoundTrip(t *testing.T) {
	pad := qpp.NewQPP([]byte("pad-seed"), 16)
	seed := []byte("session-seed")

	aliceConn, bobConn := net.Pipe()
	alice := NewQPPPort(aliceConn, pad, seed)
	bob := NewQPPPort(bobConn, pad, seed)
	t.Cleanup(func() {
		alice.Close()
		bob.Close()
	})

	t.Run("alice to bob", func(t *testing.T) {
		assertRoundTrip(t, alice, bob, []byte("encrypted hello"))
	})

	t.Run("bob to alice", func(t *testing.T) {
		assertRoundTrip(t, bob, alice, []byte("reply payload"))
	})
}

func TestQPPProtectedKeyCompatibility(t *testing.T) {
	for _, secret := range []string{"", "session-seed", "密钥"} {
		t.Run(secret, func(t *testing.T) {
			key := NewProtectedKey(secret)
			defer key.Destroy()
			protectedPad := qpp.NewQPP(key.Bytes(), 16)
			legacyPad := qpp.NewQPP([]byte(secret), 16)
			aliceConn, bobConn := net.Pipe()
			alice := NewQPPPort(aliceConn, protectedPad, key.Bytes())
			bob := NewQPPPort(bobConn, legacyPad, []byte(secret))
			defer alice.Close()
			defer bob.Close()
			assertRoundTrip(t, alice, bob, []byte("protected seed"))
			assertRoundTrip(t, bob, alice, []byte("legacy seed"))
		})
	}
}

func assertRoundTrip(t *testing.T, writer io.Writer, reader io.Reader, payload []byte) {
	t.Helper()

	recvErr := make(chan error, 1)
	go func() {
		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(reader, buf); err != nil {
			recvErr <- fmt.Errorf("read encrypted payload: %w", err)
			return
		}
		if !bytes.Equal(buf, payload) {
			recvErr <- fmt.Errorf("payload mismatch: got %q want %q", buf, payload)
			return
		}
		recvErr <- nil
	}()

	msg := append([]byte(nil), payload...)
	if n, err := writer.Write(msg); err != nil {
		t.Fatalf("write failed: %v", err)
	} else if n != len(payload) {
		t.Fatalf("write returned %d, want %d", n, len(payload))
	}

	if err := <-recvErr; err != nil {
		t.Fatalf("round trip error: %v", err)
	}
}
