// MIT License
//
// Copyright (c) 2016-2017 xtaci
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

package smux

import (
	"encoding/binary"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/transport/v5/deadline"
)

// Stream is a wrapper around stream that exposes the public API.
type Stream struct {
	*stream
}

// Stream implements net.Conn
type stream struct {
	id   uint32 // Stream identifier
	sess *Session

	bufferRing bufferRing // ring buffer for ordered incoming data

	bufferLock sync.Mutex // Mutex to protect access to buffers
	frameSize  int        // Maximum frame size for the stream

	// wakeup channels
	chReaderWakeup chan struct{}
	chWriterWakeup chan struct{}

	// stream closing
	die         chan struct{}
	dieOnce     sync.Once     // Ensures die channel is closed only once
	forcedClose atomic.Uint32 // Set when Close or Session.Close aborts the stream.

	v2WriteLock sync.Mutex // Serializes v2 window reservations between writers.

	// to handle FIN event(i.e. EOF from remote)
	chFinEvent   chan struct{}
	finEventOnce sync.Once // Ensures chFinEvent is closed only once

	// half-close support: local write closed (sent FIN)
	chWriteClosed   chan struct{}
	writeClosedOnce sync.Once // Ensures chWriteClosed is closed only once

	// read/write deadline
	readDeadline, writeDeadline *deadline.Deadline

	// v2 stream fields(flow control)
	numRead    uint32        // count num of bytes read
	numWritten atomic.Uint32 // count num of bytes written
	incr       uint32        // bytes sent since last window update

	// UPD command
	peerConsumed          atomic.Uint32 // num of bytes the peer has consumed
	peerWindow            uint32        // peer window, initialized to 256KB, updated by peer
	chUpdate              chan struct{} // notify of remote data consuming and window update
	windowUpdateThreshold uint32        // cached threshold for window update (MaxStreamBuffer/2)
}

type bufferRing struct {
	bufs  [][]byte
	heads []*[]byte
	head  int
	tail  int
	size  int
	mask  int // bitmask for fast modulo when capacity is power of 2
}

const defaultBufferRingCapacity = 8

// newBufferRing creates an empty ring with power-of-two capacity.
func newBufferRing(capacity int) bufferRing {
	if capacity < 1 {
		capacity = 1
	}
	// Round up to a power of two so ring indices can use a bitmask.
	cap := 1
	for cap < capacity {
		cap <<= 1
	}
	return bufferRing{
		bufs:  make([][]byte, cap),
		heads: make([]*[]byte, cap),
		mask:  cap - 1,
	}
}

// len returns the number of buffers currently stored in the ring.
func (r *bufferRing) len() int {
	return r.size
}

// grow doubles the ring capacity while preserving buffer order.
func (r *bufferRing) grow() {
	newCap := max(len(r.bufs)*2, 1)
	newBufs := make([][]byte, newCap)
	newHeads := make([]*[]byte, newCap)
	for i := 0; i < r.size; i++ {
		idx := (r.head + i) & r.mask
		newBufs[i] = r.bufs[idx]
		newHeads[i] = r.heads[idx]
	}
	r.bufs = newBufs
	r.heads = newHeads
	r.head = 0
	r.tail = r.size
	r.mask = newCap - 1
}

// push appends a buffer and its allocator handle to the ring.
func (r *bufferRing) push(buf []byte, head *[]byte) {
	if r.size == len(r.bufs) {
		r.grow()
	}
	r.bufs[r.tail] = buf
	r.heads[r.tail] = head
	r.tail = (r.tail + 1) & r.mask
	r.size++
}

// pop removes and returns the oldest buffer and allocator handle.
func (r *bufferRing) pop() (buf []byte, head *[]byte, ok bool) {
	if r.size == 0 {
		return nil, nil, false
	}
	buf = r.bufs[r.head]
	head = r.heads[r.head]
	r.bufs[r.head] = nil
	r.heads[r.head] = nil
	r.head = (r.head + 1) & r.mask
	r.size--
	if r.size == 0 {
		r.tail = r.head
		r.shrinkIfEmpty()
	}
	return buf, head, true
}

// shrinkIfEmpty restores the default capacity after a large ring is drained.
func (r *bufferRing) shrinkIfEmpty() {
	if r.size == 0 && len(r.bufs) > defaultBufferRingCapacity {
		*r = newBufferRing(defaultBufferRingCapacity)
	}
}

// consumeFront copies data from the front buffer into b and recycles the buffer
// when fully consumed. It returns the number of bytes copied and the recycled
// buffer, if any.
func (r *bufferRing) consumeFront(b []byte) (n int, recycled *[]byte) {
	if r.size == 0 {
		return 0, nil
	}
	n = copy(b, r.bufs[r.head])
	r.bufs[r.head] = r.bufs[r.head][n:]

	// Release the allocator buffer once all of its data has been consumed.
	if len(r.bufs[r.head]) == 0 {
		recycled = r.heads[r.head]
		r.bufs[r.head] = nil
		r.heads[r.head] = nil
		r.head = (r.head + 1) & r.mask
		r.size--
		if r.size == 0 {
			r.tail = r.head
			r.shrinkIfEmpty()
		}
	}
	return n, recycled
}

// newStream initializes the internal state for a stream.
func newStream(id uint32, frameSize int, sess *Session) *stream {
	s := new(stream)
	s.id = id
	s.chReaderWakeup = make(chan struct{}, 1)
	s.chWriterWakeup = make(chan struct{}, 1)
	s.chUpdate = make(chan struct{}, 1)
	s.frameSize = frameSize
	s.sess = sess
	s.die = make(chan struct{})
	s.chFinEvent = make(chan struct{})
	s.chWriteClosed = make(chan struct{})                             // Signals local write-side closure.
	s.peerWindow = initialPeerWindow                                  // Initial peer receive window.
	s.windowUpdateThreshold = uint32(sess.config.MaxStreamBuffer / 2) // Cached update threshold.
	// Preallocate the receive ring to reduce allocations during data transfer.
	s.bufferRing = newBufferRing(defaultBufferRingCapacity)

	s.readDeadline = deadline.New()
	s.writeDeadline = deadline.New()

	// Clear pending timers when the stream is collected instead of retaining
	// deadline resources until they expire.
	runtime.AddCleanup(s, freeDeadline, s.readDeadline)
	runtime.AddCleanup(s, freeDeadline, s.writeDeadline)

	return s
}

// ID returns the stream's unique identifier.
func (s *stream) ID() uint32 {
	return s.id
}

// Read reads data from the stream into b.
func (s *stream) Read(b []byte) (n int, err error) {
	if s.sess.config.Version == 2 {
		for {
			n, err = s.tryReadV2(b)
			if err != ErrWouldBlock {
				return n, err
			}
			if ew := s.waitRead(); ew != nil {
				if ew == io.ErrClosedPipe && s.isNormalHalfClosed() {
					continue
				}
				return 0, ew
			}
		}
	}

	for {
		n, err = s.tryReadV1(b)
		if err != ErrWouldBlock {
			return n, err
		}
		if ew := s.waitRead(); ew != nil {
			if ew == io.ErrClosedPipe && s.isNormalHalfClosed() {
				continue
			}
			return 0, ew
		}
	}
}

// tryReadV1 reads buffered data without blocking, using protocol version 1
// behavior.
func (s *stream) tryReadV1(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	// Protect the ring while copying the next available data into b.
	var recycled *[]byte
	s.bufferLock.Lock()
	n, recycled = s.bufferRing.consumeFront(b)
	s.bufferLock.Unlock()

	if recycled != nil {
		defaultAllocator.Put(recycled)
	}

	// Return shared receive capacity so recvLoop can accept more data.
	if n > 0 {
		s.sess.returnTokens(n)
		// Both FINs may have arrived while data remained buffered. Retry cleanup
		// now that these bytes have been delivered.
		s.tryHalfCloseCleanup()
		return n, nil
	}

	// Deliver buffered data before reporting EOF or a forced-close error.
	select {
	case <-s.die:
		if s.isForcedClosed() {
			return 0, io.ErrClosedPipe
		}
		return 0, io.EOF
	default:
		return 0, ErrWouldBlock
	}
}

// tryReadV2 is the non-blocking version of Read for version 2 streams.
func (s *stream) tryReadV2(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	var notifyConsumed uint32
	var recycled *[]byte
	s.bufferLock.Lock()
	n, recycled = s.bufferRing.consumeFront(b)

	// Track consumed bytes and send periodic window updates. Updating after
	// roughly half the window is consumed helps prevent a continuous reader
	// from stalling while waiting for acknowledgements.
	//
	// s.numRead == n identifies the first read from this stream.
	s.numRead += uint32(n)
	s.incr += uint32(n)

	// Send an update once the threshold is reached, and always on the first read.
	if s.incr >= s.windowUpdateThreshold || s.numRead == uint32(n) {
		notifyConsumed = s.numRead
		s.incr = 0 // Start counting toward the next update.
	}
	s.bufferLock.Unlock()

	if recycled != nil {
		defaultAllocator.Put(recycled)
	}

	if n > 0 {
		s.sess.returnTokens(n)

		// Report consumption when the update threshold has been reached.
		if notifyConsumed > 0 {
			err := s.sendWindowUpdate(notifyConsumed)
			// The bytes are already consumed, so cleanup must proceed even if
			// sending the update fails.
			s.tryHalfCloseCleanup()
			return n, err
		}
		s.tryHalfCloseCleanup()
		return n, nil
	}

	select {
	case <-s.die:
		if s.isForcedClosed() {
			return 0, io.ErrClosedPipe
		}
		return 0, io.EOF
	default:
		return 0, ErrWouldBlock
	}
}

// WriteTo implements io.WriteTo
// WriteTo writes stream data to w until the stream reaches EOF or an error occurs.
// It returns the number of bytes written and any error encountered. For protocol
// version 2, it sends window updates to the peer as needed.
func (s *stream) WriteTo(w io.Writer) (n int64, err error) {
	switch s.sess.config.Version {
	case 2:
		return s.writeToV2(w)
	default:
		return s.writeToV1(w)
	}
}

// writeToV1 drains buffered data to w without protocol version 2 updates.
func (s *stream) writeToV1(w io.Writer) (n int64, err error) {
	for {
		var buf []byte
		var head *[]byte

		// Remove the next queued buffer while holding the ring lock.
		s.bufferLock.Lock()
		if s.bufferRing.len() > 0 {
			buf, head, _ = s.bufferRing.pop()
		}
		s.bufferLock.Unlock()

		// Write the dequeued buffer to the destination.
		if buf != nil {
			nw, ew := w.Write(buf)
			// WriteTo consumes receive data just like Read, so return its tokens.
			s.sess.returnTokens(len(buf))
			defaultAllocator.Put(head)
			if nw > 0 {
				n += int64(nw)
			}

			if ew != nil {
				s.tryHalfCloseCleanup()
				return n, ew
			}
			if nw != len(buf) {
				s.tryHalfCloseCleanup()
				return n, io.ErrShortWrite
			}
			s.tryHalfCloseCleanup()
		} else if ew := s.waitRead(); ew != nil {
			if ew == io.EOF {
				if s.isForcedClosed() {
					return n, io.ErrClosedPipe
				}
				return n, nil
			}
			if ew == io.ErrClosedPipe && s.isNormalHalfClosed() {
				continue
			}
			return n, ew
		}
	}
}

// writeToV2 drains buffered data to w and reports consumed bytes to the peer.
func (s *stream) writeToV2(w io.Writer) (n int64, err error) {
	for {
		var notifyConsumed uint32
		var buf []byte
		var head *[]byte

		// Remove the next queued buffer while holding the ring lock.
		s.bufferLock.Lock()
		if s.bufferRing.len() > 0 {
			buf, head, _ = s.bufferRing.pop()
		}

		// Account for bytes consumed by WriteTo for version 2 flow control.
		var bufLen uint32
		if buf != nil {
			bufLen = uint32(len(buf))
		}
		s.numRead += bufLen
		s.incr += bufLen

		// Send an update after the threshold is reached or on the first read.
		if s.incr >= s.windowUpdateThreshold || s.numRead == bufLen {
			notifyConsumed = s.numRead
			s.incr = 0
		}
		s.bufferLock.Unlock()

		// Write the dequeued buffer to the destination.
		if buf != nil {
			nw, ew := w.Write(buf)
			// WriteTo consumes receive data just like Read, so return its tokens.
			s.sess.returnTokens(len(buf))
			defaultAllocator.Put(head)
			if nw > 0 {
				n += int64(nw)
			}

			if ew != nil {
				if notifyConsumed > 0 {
					_ = s.sendWindowUpdate(notifyConsumed)
				}
				s.tryHalfCloseCleanup()
				return n, ew
			}
			if nw != len(buf) {
				if notifyConsumed > 0 {
					_ = s.sendWindowUpdate(notifyConsumed)
				}
				s.tryHalfCloseCleanup()
				return n, io.ErrShortWrite
			}

			// Report consumed bytes so the peer can advance its send window.
			if notifyConsumed > 0 {
				if err := s.sendWindowUpdate(notifyConsumed); err != nil {
					s.tryHalfCloseCleanup()
					return n, err
				}
			}
			s.tryHalfCloseCleanup()
		} else if ew := s.waitRead(); ew != nil {
			if ew == io.EOF {
				if s.isForcedClosed() {
					return n, io.ErrClosedPipe
				}
				return n, nil
			}
			if ew == io.ErrClosedPipe && s.isNormalHalfClosed() {
				continue
			}
			return n, ew
		}
	}
}

// sendWindowUpdate reports the number of bytes consumed and the receive window to the peer.
func (s *stream) sendWindowUpdate(consumed uint32) error {
	frame := newFrame(byte(s.sess.config.Version), cmdUPD, s.id)
	var hdr updHeader
	binary.LittleEndian.PutUint32(hdr[:], consumed)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(s.sess.config.MaxStreamBuffer))
	frame.data = hdr[:]
	_, err := writeFrameInternal(s.sess, frame, s.readDeadline.Done(), CLSCTRL) // Keep flow-control updates ahead of data frames.
	return err
}

// waitRead blocks until data, closure, an I/O error, or the read deadline.
func (s *stream) waitRead() error {
	select {
	case <-s.chReaderWakeup: // A read condition may have changed.
		return nil
	case <-s.chFinEvent:
		// Check the buffer before returning EOF so queued data remains readable.
		s.bufferLock.Lock()
		defer s.bufferLock.Unlock()
		if s.bufferRing.len() > 0 {
			return nil
		}
		return io.EOF
	case <-s.sess.chSocketReadError:
		return s.sess.socketReadError
	case <-s.sess.chProtoError:
		return s.sess.protoError
	case <-s.readDeadline.Done():
		return ErrTimeout
	case <-s.die:
		if s.isNormalHalfClosed() {
			s.bufferLock.Lock()
			drained := s.bufferRing.len() == 0
			s.bufferLock.Unlock()
			if drained {
				return io.EOF
			}
			return nil
		}
		return io.ErrClosedPipe
	}

}

// checkWriteClosed reports whether the local write side or whole stream is closed.
func (s *stream) checkWriteClosed() error {
	select {
	case <-s.chWriteClosed: // The local write side was closed.
		return io.ErrClosedPipe
	case <-s.die: // The stream was fully closed.
		return io.ErrClosedPipe
	default:
		return nil
	}
}

// Write implements net.Conn
//
// Concurrent Write calls may interleave at frame boundaries, and their ordering
// is nondeterministic.
func (s *stream) Write(b []byte) (n int, err error) {
	switch s.sess.config.Version {
	case 2:
		return s.writeV2(b)
	default:
		return s.writeV1(b)
	}
}

// writeV1 splits and sends data frames without per-stream flow control.
func (s *stream) writeV1(b []byte) (n int, err error) {
	// An empty write has nothing to send.
	if len(b) == 0 {
		return 0, nil
	}

	// Reject writes after the local write side or stream has closed.
	if err := s.checkWriteClosed(); err != nil {
		return 0, err
	}

	// Split the input into frames and send them in order.
	sent := 0
	frame := newFrame(byte(s.sess.config.Version), cmdPSH, s.id)
	for len(b) > 0 {
		size := min(len(b), s.frameSize)

		frame.data = b[:size]
		s.numWritten.Add(uint32(size))
		n, err := writeFrameInternal(s.sess, frame, s.writeDeadline.Done(), CLSDATA)
		sent += n
		if err != nil {
			return sent, err
		}

		b = b[size:]
	}

	return sent, nil
}

// writeV2 sends data frames subject to the peer's advertised receive window.
func (s *stream) writeV2(b []byte) (n int, err error) {
	// An empty write has nothing to send.
	if len(b) == 0 {
		return 0, nil
	}

	// Reject writes after the local write side or stream has closed.
	if err := s.checkWriteClosed(); err != nil {
		return 0, err
	}
	s.v2WriteLock.Lock()
	defer s.v2WriteLock.Unlock()

	// Split the input into frames, respecting the peer's receive window.
	sent := 0
	frame := newFrame(byte(s.sess.config.Version), cmdPSH, s.id)

	for {
		// Calculate bytes in flight within this stream's sliding window.
		// [.... [consumed... numWritten] ... win... ]
		// [.... [consumed...................+rmtwnd]]
		// Unsigned subtraction handles counter wraparound modulo 2^32:
		// uint32(0) - uint32(math.MaxUint32) = 1
		// int32(uint32(0) - uint32(1)) = -1
		//
		// The signed result distinguishes a valid in-flight count from an
		// invalid peer-consumed count.
		inflight := int32(s.numWritten.Load() - s.peerConsumed.Load())
		if inflight < 0 { // Reject an inconsistent peer-consumed count.
			return 0, ErrConsumed
		}

		// Compute remaining window space using the same modulo-2^32 counters.
		win := int32(atomic.LoadUint32(&s.peerWindow)) - inflight

		if win > 0 {
			// Limit this batch to the available window space.
			n := min(len(b), int(win))

			// Split the batch into frames and send them in order.
			bts := b[:n]
			for len(bts) > 0 {
				// Keep each frame within the configured maximum size.
				size := min(len(bts), s.frameSize)
				frame.data = bts[:size]

				s.numWritten.Add(uint32(size))
				nw, err := writeFrameInternal(s.sess, frame, s.writeDeadline.Done(), CLSDATA)
				sent += nw
				if err != nil {
					return sent, err
				}

				bts = bts[size:]
			}

			b = b[n:]
		}

		// Return once all input bytes have been sent.
		if len(b) <= 0 {
			return sent, nil
		}

		// Wait for more window space, closure, or the write deadline. Blocking
		// here propagates flow control to the caller as backpressure.
		select {
		case <-s.chWriterWakeup: // The stream's writable state may have changed.
		case <-s.chWriteClosed: // The local write side was half-closed.
			return sent, io.ErrClosedPipe
		case <-s.die:
			return sent, io.ErrClosedPipe
		case <-s.writeDeadline.Done():
			return sent, ErrTimeout
		case <-s.sess.chSocketWriteError:
			return sent, s.sess.socketWriteError
		case <-s.chUpdate: // The peer consumed data or updated its receive window.
			continue
		}
	}
}

// CloseWrite implements half-close by closing the write side of the stream.
// After CloseWrite, the stream can still receive data from the peer,
// but any further writes will return io.ErrClosedPipe.
// This is similar to net.TCPConn.CloseWrite().
func (s *stream) CloseWrite() error {
	var once bool
	s.writeClosedOnce.Do(func() {
		close(s.chWriteClosed)
		once = true
	})

	if !once {
		return io.ErrClosedPipe
	}

	// Notify the peer that no more data will be written on this stream.
	f := newFrame(byte(s.sess.config.Version), cmdFIN, s.id)

	timer := time.NewTimer(openCloseTimeout)
	defer timer.Stop()

	_, err := writeFrameInternal(s.sess, f, timer.C, CLSDATA)
	s.tryHalfCloseCleanup()
	return err
}

// Close implements net.Conn
// Close fully closes the stream (both read and write sides).
func (s *stream) Close() error {
	s.forcedClose.Store(1)

	var once bool
	s.dieOnce.Do(func() {
		close(s.die)
		once = true
	})

	if !once {
		// Half-close cleanup may have closed die while unread data kept the
		// stream registered. An explicit Close discards that data, so remove
		// the stream and reclaim its buffers now.
		s.sess.streamClosed(s.id)
		return io.ErrClosedPipe
	}

	// Mark the local write side closed if it has not already been closed.
	s.writeClosedOnce.Do(func() {
		close(s.chWriteClosed)
	})

	// Send FIN after prior data frames so the peer observes them in order.
	f := newFrame(byte(s.sess.config.Version), cmdFIN, s.id)

	timer := time.NewTimer(openCloseTimeout)
	defer timer.Stop()

	_, err := writeFrameInternal(s.sess, f, timer.C, CLSDATA) // Order FIN with the stream's data frames.
	s.sess.streamClosed(s.id)
	return err
}

// GetDieCh returns a channel that is closed when the stream is closed.
func (s *stream) GetDieCh() <-chan struct{} {
	return s.die
}

// SetReadDeadline sets the read deadline as defined by
// net.Conn.SetReadDeadline.
// A zero time value disables the deadline.
func (s *stream) SetReadDeadline(t time.Time) error {
	s.readDeadline.Set(t)
	s.wakeupReader()
	return nil
}

// SetWriteDeadline sets the write deadline as defined by
// net.Conn.SetWriteDeadline.
// A zero time value disables the deadline.
func (s *stream) SetWriteDeadline(t time.Time) error {
	s.writeDeadline.Set(t)
	s.wakeupWriter()
	return nil
}

// SetDeadline sets both read and write deadlines as defined by
// net.Conn.SetDeadline.
// A zero time value disables the deadlines.
func (s *stream) SetDeadline(t time.Time) error {
	if err := s.SetReadDeadline(t); err != nil {
		return err
	}
	if err := s.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

// sessionClose closes the stream when its session shuts down.
func (s *stream) sessionClose() {
	s.forcedClose.Store(1)
	s.dieOnce.Do(func() { close(s.die) })
}

// LocalAddr returns the underlying connection's local address, if available.
func (s *stream) LocalAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the underlying connection's remote address, if available.
func (s *stream) RemoteAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface {
		RemoteAddr() net.Addr
	}); ok {
		return ts.RemoteAddr()
	}
	return nil
}

// pushBytes appends buf to the stream's receive buffer.
func (s *stream) pushBytes(pbuf *[]byte) {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()
	s.bufferRing.push(*pbuf, pbuf)
}

// recycleTokens returns the number of unread bytes and releases their buffers.
func (s *stream) recycleTokens() (n int) {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()
	for s.bufferRing.len() > 0 {
		buf, head, _ := s.bufferRing.pop()
		n += len(buf)
		defaultAllocator.Put(head)
	}
	return
}

// wakeupReader signals a blocked reader that stream state may have changed.
func (s *stream) wakeupReader() {
	select {
	case s.chReaderWakeup <- struct{}{}:
	default:
	}
}

// wakeupWriter signals a blocked writer that stream state may have changed.
func (s *stream) wakeupWriter() {
	select {
	case s.chWriterWakeup <- struct{}{}:
	default:
	}
}

// update records the peer's consumed-byte count and receive-window size.
func (s *stream) update(consumed uint32, window uint32) {
	// Publish both flow-control values before waking a blocked writer.
	s.peerConsumed.Store(consumed)
	atomic.StoreUint32(&s.peerWindow, window)

	// Wake a writer waiting for window space or a peer update.
	select {
	case s.chUpdate <- struct{}{}:
	default:
	}
}

// fin records that the peer has closed its write side by sending FIN.
func (s *stream) fin() {
	s.finEventOnce.Do(func() {
		close(s.chFinEvent)
	})
	s.tryHalfCloseCleanup()
}

// tryHalfCloseCleanup closes the stream after both sides send FIN and removes
// it from the session once its buffered data has been consumed.
func (s *stream) tryHalfCloseCleanup() {
	select {
	case <-s.chFinEvent:
	default:
		return
	}

	select {
	case <-s.chWriteClosed:
	default:
		return
	}

	s.dieOnce.Do(func() {
		close(s.die)
	})

	// Closing die wakes readers so they can observe EOF, but must not discard
	// bytes that recvLoop queued before the peer's FIN. Keep the stream in the
	// session map until those bytes are consumed; streamClosed recycles any
	// remainder, so it is safe to call only after the ring is empty.
	s.bufferLock.Lock()
	drained := s.bufferRing.len() == 0
	s.bufferLock.Unlock()
	if drained {
		s.sess.streamClosed(s.id)
	}
}

// isNormalHalfClosed reports whether both peers have closed their write sides
// without a forced close.
func (s *stream) isNormalHalfClosed() bool {
	if s.isForcedClosed() {
		return false
	}

	select {
	case <-s.chFinEvent:
	default:
		return false
	}

	select {
	case <-s.chWriteClosed:
	default:
		return false
	}

	return true
}

// isForcedClosed reports whether the stream was closed explicitly or by its session.
func (s *stream) isForcedClosed() bool {
	return s.forcedClose.Load() != 0
}
