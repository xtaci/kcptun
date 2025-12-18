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
