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
	"sync"
	"sync/atomic"
	"time"
)

// wrapper for GC
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
	dieOnce     sync.Once // Ensures die channel is closed only once
	forcedClose uint32    // Set when Close or Session.Close aborts the stream.

	// to handle FIN event(i.e. EOF from remote)
	chFinEvent   chan struct{}
	finEventOnce sync.Once // Ensures chFinEvent is closed only once

	// half-close support: local write closed (sent FIN)
	chWriteClosed   chan struct{}
	writeClosedOnce sync.Once // Ensures chWriteClosed is closed only once

	// read/write deadline
	readDeadline  atomic.Value
	writeDeadline atomic.Value

	// v2 stream fields(flow control)
	numRead    uint32 // count num of bytes read
	numWritten uint32 // count num of bytes written
	incr       uint32 // bytes sent since last window update

	// UPD command
	peerConsumed          uint32        // num of bytes the peer has consumed
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

func newBufferRing(capacity int) bufferRing {
	if capacity < 1 {
		capacity = 1
	}
	// ensure capacity is power of 2 for fast modulo using bitmask
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

func (r *bufferRing) len() int {
	return r.size
}

func (r *bufferRing) grow() {
	newCap := len(r.bufs) * 2
	if newCap < 1 {
		newCap = 1
	}
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

func (r *bufferRing) push(buf []byte, head *[]byte) {
	if r.size == len(r.bufs) {
		r.grow()
	}
	r.bufs[r.tail] = buf
	r.heads[r.tail] = head
	r.tail = (r.tail + 1) & r.mask
	r.size++
}

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

func (r *bufferRing) shrinkIfEmpty() {
	if r.size == 0 && len(r.bufs) > defaultBufferRingCapacity {
		*r = newBufferRing(defaultBufferRingCapacity)
	}
}

// consumeFront copies data from the front buffer to b, recycles the buffer if fully consumed,
// and returns the number of bytes copied. Returns 0 if the ring is empty.
func (r *bufferRing) consumeFront(b []byte) (n int, recycled *[]byte) {
	if r.size == 0 {
		return 0, nil
	}
	n = copy(b, r.bufs[r.head])
	r.bufs[r.head] = r.bufs[r.head][n:]

	// recycle buffer when fully consumed
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

// newStream initializes and returns a new Stream.
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
	s.chWriteClosed = make(chan struct{})                             // half-close support
	s.peerWindow = initialPeerWindow                                  // set to initial window size
	s.windowUpdateThreshold = uint32(sess.config.MaxStreamBuffer / 2) // cache threshold
	// pre-allocate ring buffer to reduce allocations during data transfer
	s.bufferRing = newBufferRing(defaultBufferRingCapacity)

	return s
}

// ID returns the stream's unique identifier.
func (s *stream) ID() uint32 {
	return s.id
}

// Read reads data from the stream into the provided buffer.
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

func (s *stream) tryReadV1(b []byte) (n int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	// A critical section to copy data from buffers to b
	var recycled *[]byte
	s.bufferLock.Lock()
	n, recycled = s.bufferRing.consumeFront(b)
	s.bufferLock.Unlock()

	if recycled != nil {
		defaultAllocator.Put(recycled)
	}

	// return tokens to session to allow more data to be received
	if n > 0 {
		s.sess.returnTokens(n)
		// Both FINs may have arrived while this data was buffered. Retry
		// cleanup after consuming it so the stream is removed as soon as the
		// final buffered bytes have been delivered.
		s.tryHalfCloseCleanup()
		return n, nil
	}

	// even if the stream has been closed, we try to deliver all buffered data first.
	// only when there's no data left in buffer, we return EOF to reader.
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

	// In an ideal environment:
	// If more than half of the buffer has been consumed, send a read ACK to the peer.
	// With the ACK round-trip time taken into account, a continuous data stream
	// will not slow down due to waiting for ACKs, as long as the consumer
	// continues reading data.
	//
	// s.numRead == n indicates that this is the initial read.
	s.numRead += uint32(n)
	s.incr += uint32(n)

	// send window update if the increased bytes exceed half of the buffer size
	// or this is the initial read.
	if s.incr >= s.windowUpdateThreshold || s.numRead == uint32(n) {
		notifyConsumed = s.numRead
		s.incr = 0 // reset incr counter
	}
	s.bufferLock.Unlock()

	if recycled != nil {
		defaultAllocator.Put(recycled)
	}

	if n > 0 {
		s.sess.returnTokens(n)

		// send window update if necessary
		if notifyConsumed > 0 {
			err := s.sendWindowUpdate(notifyConsumed)
			// A window update can fail after bytes have already been consumed;
			// cleanup must not depend on the update succeeding.
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
// WriteTo writes data to w until there's no more data to write or when an error occurs.
// The return value n is the number of bytes written. Any error encountered during the write is also returned.
// WriteTo calls Write in a loop until there is no more data to write or when an error occurs.
// If the underlying stream is a v2 stream, it will send window update to peer when necessary.
// If the underlying stream is a v1 stream, it will not send window update to peer.
func (s *stream) WriteTo(w io.Writer) (n int64, err error) {
	switch s.sess.config.Version {
	case 2:
		return s.writeToV2(w)
	default:
		return s.writeToV1(w)
	}
}

// check comments in WriteTo
func (s *stream) writeToV1(w io.Writer) (n int64, err error) {
	for {
		var buf []byte
		var head *[]byte

		// get the next buffer to write
		s.bufferLock.Lock()
		if s.bufferRing.len() > 0 {
			buf, head, _ = s.bufferRing.pop()
		}
		s.bufferLock.Unlock()

		// write the buffer to w
		if buf != nil {
			nw, ew := w.Write(buf)
			// NOTE: WriteTo is a reader, so we need to return tokens here
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

// check comments in WriteTo
func (s *stream) writeToV2(w io.Writer) (n int64, err error) {
	for {
		var notifyConsumed uint32
		var buf []byte
		var head *[]byte

		// get the next buffer to write
		s.bufferLock.Lock()
		if s.bufferRing.len() > 0 {
			buf, head, _ = s.bufferRing.pop()
		}

		// in v2, we need to track the number of bytes read
		var bufLen uint32
		if buf != nil {
			bufLen = uint32(len(buf))
		}
		s.numRead += bufLen
		s.incr += bufLen

		// send window update if the increased bytes exceed half of the buffer size
		if s.incr >= s.windowUpdateThreshold || s.numRead == bufLen {
			notifyConsumed = s.numRead
			s.incr = 0
		}
		s.bufferLock.Unlock()

		// same as v1, write the buffer to w
		if buf != nil {
			nw, ew := w.Write(buf)
			// NOTE: WriteTo is a reader, so we need to return tokens here
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

			// send window update
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

// sendWindowUpdate sends a window update command to the peer.
func (s *stream) sendWindowUpdate(consumed uint32) error {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := s.readDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	frame := newFrame(byte(s.sess.config.Version), cmdUPD, s.id)
	var hdr updHeader
	binary.LittleEndian.PutUint32(hdr[:], consumed)
	binary.LittleEndian.PutUint32(hdr[4:], uint32(s.sess.config.MaxStreamBuffer))
	frame.data = hdr[:]
	_, err := s.sess.writeFrameInternal(frame, deadline, CLSCTRL) // <-- NOTE(x): use control channel
	return err
}

// waitRead blocks until a read event occurs or a deadline is reached.
func (s *stream) waitRead() error {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := s.readDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case <-s.chReaderWakeup: // notify some data has arrived, or closed
		return nil
	case <-s.chFinEvent:
		// BUGFIX(xtaci): Fix for https://github.com/xtaci/smux/issues/82
		s.bufferLock.Lock()
		defer s.bufferLock.Unlock()
		if s.bufferRing.len() > 0 {
			return nil
		}
		return io.EOF
	case <-s.sess.chSocketReadError:
		return s.sess.socketReadError.Load().(error)
	case <-s.sess.chProtoError:
		return s.sess.protoError.Load().(error)
	case <-deadline:
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

// checkWriteClosed checks if the stream write side has been closed.
// Returns io.ErrClosedPipe if closed, nil otherwise.
func (s *stream) checkWriteClosed() error {
	select {
	case <-s.chWriteClosed: // local write closed (half-close)
		return io.ErrClosedPipe
	case <-s.die: // full close
		return io.ErrClosedPipe
	default:
		return nil
	}
}

// Write implements net.Conn
//
// Note that the behavior when multiple goroutines write concurrently is not deterministic,
// frames may interleave in random way.
func (s *stream) Write(b []byte) (n int, err error) {
	switch s.sess.config.Version {
	case 2:
		return s.writeV2(b)
	default:
		return s.writeV1(b)
	}
}

// writeV1 writes data to the stream for version 1 streams.
func (s *stream) writeV1(b []byte) (n int, err error) {
	// check empty input
	if len(b) == 0 {
		return 0, nil
	}

	// check if stream write side has closed
	if err := s.checkWriteClosed(); err != nil {
		return 0, err
	}

	// create write deadline timer
	var deadline <-chan time.Time
	if d, ok := s.writeDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer := time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	// frame split and transmit
	sent := 0
	frame := newFrame(byte(s.sess.config.Version), cmdPSH, s.id)
	for len(b) > 0 {
		size := len(b)
		if size > s.frameSize {
			size = s.frameSize
		}

		frame.data = b[:size]
		n, err := s.sess.writeFrameInternal(frame, deadline, CLSDATA)
		atomic.AddUint32(&s.numWritten, uint32(size))
		sent += n
		if err != nil {
			return sent, err
		}

		b = b[size:]
	}

	return sent, nil
}

// writeV2 writes data to the stream for version 2 streams.
func (s *stream) writeV2(b []byte) (n int, err error) {
	// check empty input
	if len(b) == 0 {
		return 0, nil
	}

	// check if stream write side has closed
	if err := s.checkWriteClosed(); err != nil {
		return 0, err
	}

	// frame split and transmit process
	sent := 0
	frame := newFrame(byte(s.sess.config.Version), cmdPSH, s.id)

	var deadlineTimer *time.Timer
	defer func() {
		stopTimer(deadlineTimer)
	}()

	for {
		deadline := (<-chan time.Time)(nil)
		if d, ok := s.writeDeadline.Load().(time.Time); ok && !d.IsZero() {
			dur := time.Until(d)
			if dur < 0 {
				dur = 0
			}
			if deadlineTimer == nil {
				deadlineTimer = time.NewTimer(dur)
			} else {
				stopTimer(deadlineTimer)
				deadlineTimer.Reset(dur)
			}
			deadline = deadlineTimer.C
		} else if deadlineTimer != nil {
			stopTimer(deadlineTimer)
			deadlineTimer = nil
		}

		// per stream sliding window control
		// [.... [consumed... numWritten] ... win... ]
		// [.... [consumed...................+rmtwnd]]
		// note:
		// even if uint32 overflow, this math still works:
		// eg1: uint32(0) - uint32(math.MaxUint32) = 1
		// eg2: int32(uint32(0) - uint32(1)) = -1
		//
		// basically, you can take it as a MODULAR ARITHMETIC
		inflight := int32(atomic.LoadUint32(&s.numWritten) - atomic.LoadUint32(&s.peerConsumed))
		if inflight < 0 { // security check for malformed data
			return 0, ErrConsumed
		}

		// make sure you understand 'win' is calculated in modular arithmetic(2^32(4GB))
		win := int32(atomic.LoadUint32(&s.peerWindow)) - inflight

		if win > 0 {
			// determine how many bytes to send
			n := len(b)
			if n > int(win) {
				n = int(win)
			}

			// frame split and transmit
			bts := b[:n]
			for len(bts) > 0 {
				// splitting frame
				size := len(bts)
				if size > s.frameSize {
					size = s.frameSize
				}
				frame.data = bts[:size]

				// transmit of frame
				nw, err := s.sess.writeFrameInternal(frame, deadline, CLSDATA)
				atomic.AddUint32(&s.numWritten, uint32(size))
				sent += nw
				if err != nil {
					return sent, err
				}

				bts = bts[size:]
			}

			b = b[n:]
		}

		// all data has been sent
		if len(b) <= 0 {
			return sent, nil
		}

		// If there is remaining data to be sent,
		// wait until the stream is closed, the window changes, or the deadline is reached.
		// This blocking behavior propagates flow control back to the upper layer (backpressure).
		select {
		case <-s.chWriterWakeup: // wakeup
		case <-s.chWriteClosed: // local write closed (half-close)
			return sent, io.ErrClosedPipe
		case <-s.die:
			return sent, io.ErrClosedPipe
		case <-deadline:
			return sent, ErrTimeout
		case <-s.sess.chSocketWriteError:
			return sent, s.sess.socketWriteError.Load().(error)
		case <-s.chUpdate: // notify of remote data consuming and window update
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

	// send FIN to notify the peer that we are done writing
	f := newFrame(byte(s.sess.config.Version), cmdFIN, s.id)

	timer := time.NewTimer(openCloseTimeout)
	defer timer.Stop()

	_, err := s.sess.writeFrameInternal(f, timer.C, CLSDATA)
	s.tryHalfCloseCleanup()
	return err
}

// Close implements net.Conn
// Close fully closes the stream (both read and write sides).
func (s *stream) Close() error {
	atomic.StoreUint32(&s.forcedClose, 1)

	var once bool
	s.dieOnce.Do(func() {
		close(s.die)
		once = true
	})

	if !once {
		// The stream may have been marked dead by half-close cleanup while
		// unread data kept it in the session map. A later explicit Close means
		// the caller is discarding that data, so still reclaim the stream.
		s.sess.streamClosed(s.id)
		return io.ErrClosedPipe
	}

	// also close the write side if not already closed
	s.writeClosedOnce.Do(func() {
		close(s.chWriteClosed)
	})

	// send FIN in order
	f := newFrame(byte(s.sess.config.Version), cmdFIN, s.id)

	timer := time.NewTimer(openCloseTimeout)
	defer timer.Stop()

	_, err := s.sess.writeFrameInternal(f, timer.C, CLSDATA) // NOTE(x): use data channel, EOF as data.
	s.sess.streamClosed(s.id)
	return err
}

// GetDieCh returns a readonly chan which can be readable
// when the stream is to be closed.
func (s *stream) GetDieCh() <-chan struct{} {
	return s.die
}

// SetReadDeadline sets the read deadline as defined by
// net.Conn.SetReadDeadline.
// A zero time value disables the deadline.
func (s *stream) SetReadDeadline(t time.Time) error {
	s.readDeadline.Store(t)
	s.wakeupReader()
	return nil
}

// SetWriteDeadline sets the write deadline as defined by
// net.Conn.SetWriteDeadline.
// A zero time value disables the deadline.
func (s *stream) SetWriteDeadline(t time.Time) error {
	s.writeDeadline.Store(t)
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

// session closes
func (s *stream) sessionClose() {
	atomic.StoreUint32(&s.forcedClose, 1)
	s.dieOnce.Do(func() { close(s.die) })
}

// LocalAddr satisfies net.Conn interface
func (s *stream) LocalAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

// RemoteAddr satisfies net.Conn interface
func (s *stream) RemoteAddr() net.Addr {
	if ts, ok := s.sess.conn.(interface {
		RemoteAddr() net.Addr
	}); ok {
		return ts.RemoteAddr()
	}
	return nil
}

// pushBytes append buf to buffers
func (s *stream) pushBytes(pbuf *[]byte) {
	s.bufferLock.Lock()
	defer s.bufferLock.Unlock()
	s.bufferRing.push(*pbuf, pbuf)
}

// recycleTokens transform remaining bytes to tokens(will truncate buffer)
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

// wakeupReader notifies read process
func (s *stream) wakeupReader() {
	select {
	case s.chReaderWakeup <- struct{}{}:
	default:
	}
}

// wakeupWriter notifies write process
func (s *stream) wakeupWriter() {
	select {
	case s.chWriterWakeup <- struct{}{}:
	default:
	}
}

// update command
func (s *stream) update(consumed uint32, window uint32) {
	// update peer consumed and window size immediately
	atomic.StoreUint32(&s.peerConsumed, consumed)
	atomic.StoreUint32(&s.peerWindow, window)

	// notify write process
	select {
	case s.chUpdate <- struct{}{}:
	default:
	}
}

// mark this stream has been closed in protocol, i.e. receive EOF
func (s *stream) fin() {
	s.finEventOnce.Do(func() {
		close(s.chFinEvent)
	})
	s.tryHalfCloseCleanup()
}

// tryHalfCloseCleanup removes stream after both sides have sent FIN.
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

func (s *stream) isForcedClosed() bool {
	return atomic.LoadUint32(&s.forcedClose) != 0
}

// stopTimer stops the supplied timer and drains its channel if needed.
func stopTimer(t *time.Timer) {
	if t == nil {
		return
	}
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
