package std

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
)

type writerToStub struct {
	data          []byte
	writeToCalled bool
	readCalled    bool
}

func (w *writerToStub) Read(p []byte) (int, error) {
	w.readCalled = true
	return copy(p, w.data), io.EOF
}

func (w *writerToStub) WriteTo(dst io.Writer) (int64, error) {
	w.writeToCalled = true
	n, err := dst.Write(w.data)
	return int64(n), err
}

type readerFromStub struct {
	bytes.Buffer
	readFromCalled bool
}

func (r *readerFromStub) ReadFrom(src io.Reader) (int64, error) {
	r.readFromCalled = true
	return r.Buffer.ReadFrom(src)
}

type noWriterToReader struct {
	data   []byte
	offset int
}

func (r *noWriterToReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func TestCopyPrefersWriterTo(t *testing.T) {
	src := &writerToStub{data: []byte("hello world")}
	var dst bytes.Buffer

	n, err := Copy(&dst, src)
	if err != nil {
		t.Fatalf("Copy returned error: %v", err)
	}
	if n != int64(len(src.data)) {
		t.Fatalf("Copy returned %d, want %d", n, len(src.data))
	}
	if !src.writeToCalled {
		t.Fatalf("WriteTo was not used")
	}
	if src.readCalled {
		t.Fatalf("Read should not be called when WriteTo is available")
	}
	if got := dst.String(); got != string(src.data) {
		t.Fatalf("unexpected dst: %q", got)
	}
}

func TestCopyPrefersReaderFrom(t *testing.T) {
	src := &noWriterToReader{data: []byte("reader from data")}
	dst := &readerFromStub{}

	n, err := Copy(dst, src)
	if err != nil {
		t.Fatalf("Copy returned error: %v", err)
	}
	if n != int64(len("reader from data")) {
		t.Fatalf("Copy returned %d, want %d", n, len("reader from data"))
	}
	if !dst.readFromCalled {
		t.Fatalf("ReadFrom was not used")
	}
	if got := dst.String(); got != "reader from data" {
		t.Fatalf("unexpected dst: %q", got)
	}
}

func TestPipeBidirectional(t *testing.T) {
	aliceClient, aliceServer := net.Pipe()
	bobClient, bobServer := net.Pipe()

	done := make(chan error, 3)
	go func() {
		errA, errB := Pipe(aliceServer, bobServer, 0)
		if errA != nil && !errors.Is(errA, io.ErrClosedPipe) {
			done <- errA
			return
		}
		if errB != nil && !errors.Is(errB, io.ErrClosedPipe) {
			done <- errB
			return
		}
		done <- nil
	}()

	msgAB := []byte("hello bob")
	recvAB := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(msgAB))
		if _, err := io.ReadFull(bobClient, buf); err != nil {
			done <- err
			return
		}
		recvAB <- buf
	}()

	if _, err := aliceClient.Write(msgAB); err != nil {
		t.Fatalf("alice write: %v", err)
	}

	msgBA := []byte("hi alice")
	recvBA := make(chan []byte, 1)
	go func() {
		buf := make([]byte, len(msgBA))
		if _, err := io.ReadFull(aliceClient, buf); err != nil {
			done <- err
			return
		}
		recvBA <- buf
	}()

	if _, err := bobClient.Write(msgBA); err != nil {
		t.Fatalf("bob write: %v", err)
	}

	if got := <-recvAB; !bytes.Equal(got, msgAB) {
		t.Fatalf("alice->bob payload mismatch: %q", got)
	}
	if got := <-recvBA; !bytes.Equal(got, msgBA) {
		t.Fatalf("bob->alice payload mismatch: %q", got)
	}

	aliceClient.Close()
	bobClient.Close()

	if err := <-done; err != nil {
		t.Fatalf("pipe error: %v", err)
	}
}
