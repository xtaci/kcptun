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
	"errors"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/transport/v5/deadline"
)

const (
	defaultAcceptBacklog = 1024
	minShaperNotifySize  = 16
	maxShaperSize        = 1024
	openCloseTimeout     = 30 * time.Second // Timeout for opening/closing streams
)

// resultChanPool reduces allocation of result channels
var resultChanPool = sync.Pool{
	New: func() any {
		return make(chan writeResult, 1)
	},
}

// CLASSID identifies a frame's scheduling class.
type CLASSID int

const (
	CLSCTRL CLASSID = iota // prioritized control signal
	CLSDATA
)

// timeoutError represents a timeout during an operation such as accept, read, or write.
//
// To better cooperate with the standard library, timeoutError should implement the standard library's `net.Error`.
//
// For example, using smux to implement net.Listener and work with http.Server, the keep-alive connection (*smux.Stream) will be unexpectedly closed.
// For more details, see https://github.com/xtaci/smux/pull/99.
type timeoutError struct{}

// Error returns the timeout error message.
func (timeoutError) Error() string { return "timeout" }

// Temporary reports that the timeout is temporary.
func (timeoutError) Temporary() bool { return true }

// Timeout reports that the error was caused by a timeout.
func (timeoutError) Timeout() bool { return true }

var (
	ErrInvalidProtocol           = errors.New("invalid protocol")
	ErrConsumed                  = errors.New("peer consumed more than sent")
	ErrGoAway                    = errors.New("stream id overflows, should start a new connection")
	ErrTimeout         net.Error = &timeoutError{}
	ErrWouldBlock                = errors.New("operation would block on IO")
)

// writeRequest represents a frame waiting to be written by sendLoop.
type writeRequest struct {
	class  CLASSID
	frame  Frame
	seq    uint32
	result chan writeResult
}

// writeResult records the payload bytes written and any resulting error.
type writeResult struct {
	n   int
	err error
}

// Session manages streams multiplexed over a single connection.
type Session struct {
	conn io.ReadWriteCloser

	config           *Config
	goAway           int32  // flag id exhausted
	nextStreamID     uint32 // next stream identifier
	nextStreamIDLock sync.Mutex

	bucket       int32         // token bucket
	bucketNotify chan struct{} // used for waiting for tokens

	streams    map[uint32]*stream // all streams in this session
	streamLock sync.Mutex         // locks streams

	die     chan struct{} // flag session has died
	dieOnce sync.Once
	closed  atomic.Int32 // atomic flag for fast IsClosed check

	// socket error handling
	socketReadError      error
	socketWriteError     error
	chSocketReadError    chan struct{}
	chSocketWriteError   chan struct{}
	socketReadErrorOnce  sync.Once
	socketWriteErrorOnce sync.Once

	// smux protocol errors
	protoError     error
	chProtoError   chan struct{}
	protoErrorOnce sync.Once

	chAccepts chan *stream

	sessionIsActive atomic.Int32       // flag session is active
	acceptDeadline  *deadline.Deadline // deadline for Accept()

	requestID        atomic.Uint32     // Monotonic increasing write request ID
	writeReqLock     sync.RWMutex      // fences write request admission during shutdown
	shaper           chan writeRequest // a shaper for writing
	sq               *shaperQueue
	chShaperDone     chan struct{}
	chShaperPending  chan struct{}
	chShaperConsumed chan struct{}
}

// newSession creates a session and starts its background processing loops.
func newSession(config *Config, conn io.ReadWriteCloser, client bool) *Session {
	s := new(Session)
	s.die = make(chan struct{})
	s.conn = conn
	s.config = config
	s.streams = make(map[uint32]*stream)
	s.chAccepts = make(chan *stream, defaultAcceptBacklog)
	s.bucket = int32(config.MaxReceiveBuffer)
	s.bucketNotify = make(chan struct{}, 1)
	s.shaper = make(chan writeRequest, maxShaperSize)
	s.chSocketReadError = make(chan struct{})
	s.chSocketWriteError = make(chan struct{})
	s.chProtoError = make(chan struct{})
	s.chShaperPending = make(chan struct{}, 1)
	s.chShaperConsumed = make(chan struct{}, 1)
	s.chShaperDone = make(chan struct{})
	s.sq = NewShaperQueue()

	s.nextStreamID = 0
	if client {
		s.nextStreamID = 1
	}

	s.acceptDeadline = deadline.New()
	// Release the accept-deadline timer when the session is collected.
	runtime.AddCleanup(s, freeDeadline, s.acceptDeadline)

	go s.shaperLoop()
	go s.recvLoop()
	go s.sendLoop()
	if !config.KeepAliveDisabled {
		go s.keepalive()
	}
	return s
}

// allocStreamID allocates the next stream ID, or reports exhaustion.
func (s *Session) allocStreamID() (uint32, bool) {
	s.nextStreamIDLock.Lock()
	defer s.nextStreamIDLock.Unlock()

	if s.goAway > 0 {
		return 0, false
	}
	// Detect wraparound before advancing to the next stream ID.
	if s.nextStreamID+2 < s.nextStreamID {
		s.goAway = 1
		return 0, false
	}
	// Reserve the next ID in this endpoint's sequence.
	s.nextStreamID += 2
	return s.nextStreamID, true
}

// OpenStream opens a new stream on the session.
func (s *Session) OpenStream() (*Stream, error) {
	if s.IsClosed() {
		return nil, io.ErrClosedPipe
	}

	sid, ok := s.allocStreamID()
	if !ok {
		return nil, ErrGoAway
	}

	stream := newStream(sid, s.config.MaxFrameSize, s)

	// Register the stream before sending SYN so an immediate peer response is
	// not discarded by recvLoop as belonging to an unknown stream.
	s.streamLock.Lock()
	s.streams[sid] = stream
	s.streamLock.Unlock()

	_, err := s.writeControlFrame(newFrame(byte(s.config.Version), cmdSYN, sid))
	if err != nil {
		goto cleanup
	}

	select {
	case <-s.chSocketReadError:
		err = s.socketReadError
	case <-s.chSocketWriteError:
		err = s.socketWriteError
	case <-s.die:
		err = io.ErrClosedPipe
	default:
		wrapper := &Stream{stream: stream}
		// Do not install a finalizer; it could close a stream still in use.
		/*
			runtime.SetFinalizer(wrapper, func(s *Stream) {
				s.Close()
			})
		*/
		return wrapper, nil
	}

cleanup:
	s.streamClosed(sid)
	return nil, err
}

// Open opens a stream and returns it as an io.ReadWriteCloser.
func (s *Session) Open() (io.ReadWriteCloser, error) {
	return s.OpenStream()
}

// AcceptStream waits for and returns the next incoming stream.
func (s *Session) AcceptStream() (*Stream, error) {
	if s.IsClosed() {
		return nil, io.ErrClosedPipe
	}

	select {
	case stream := <-s.chAccepts:
		if s.IsClosed() {
			return nil, io.ErrClosedPipe
		}
		wrapper := &Stream{stream: stream}
		// Do not install a finalizer: io.Copy may call the embedded stream's
		// WriteTo method, making this wrapper unreachable while the transfer is
		// still active. A finalizer could then close the stream prematurely.
		return wrapper, nil
	case <-s.acceptDeadline.Done():
		return nil, ErrTimeout
	case <-s.chSocketReadError:
		return nil, s.socketReadError
	case <-s.chProtoError:
		return nil, s.protoError
	case <-s.die:
		return nil, io.ErrClosedPipe
	}
}

// Accept returns the next stream as a generic io.ReadWriteCloser.
func (s *Session) Accept() (io.ReadWriteCloser, error) {
	return s.AcceptStream()
}

// closeStreams closes every registered stream and reclaims its receive tokens.
func (s *Session) closeStreams() {
	s.streamLock.Lock()
	defer s.streamLock.Unlock()

	for k, stream := range s.streams {
		stream.sessionClose()
		if n := stream.recycleTokens(); n > 0 {
			atomic.AddInt32(&s.bucket, int32(n))
		}
		delete(s.streams, k)
	}
}

// Close is used to close the session and all streams.
func (s *Session) Close() error {
	var once bool
	s.dieOnce.Do(func() {
		s.closed.Store(1)
		close(s.die)
		once = true
	})

	if !once {
		return io.ErrClosedPipe
	}

	// Wait for in-flight admissions to finish before the shaper drains its queues.
	s.writeReqLock.Lock()
	s.writeReqLock.Unlock()

	s.closeStreams()
	err := s.conn.Close()
	if s.chShaperDone != nil {
		<-s.chShaperDone
	}
	// Drain again because an admission racing with shutdown may have completed
	// after shaperLoop's initial drain.
	s.discardPendingWrites(io.ErrClosedPipe)
	s.drainAccepts()
	return err
}

// drainAccepts removes any streams still waiting in the accept queue.
func (s *Session) drainAccepts() {
	for {
		select {
		case <-s.chAccepts:
		default:
			return
		}
	}
}

// CloseChan returns a channel that is closed when the session shuts down.
func (s *Session) CloseChan() <-chan struct{} {
	return s.die
}

// notifyBucket wakes recvLoop after receive-buffer capacity becomes available.
func (s *Session) notifyBucket() {
	select {
	case s.bucketNotify <- struct{}{}:
	default:
	}
}

// notifyReadError records the first underlying-connection read error.
func (s *Session) notifyReadError(err error) {
	s.socketReadErrorOnce.Do(func() {
		s.socketReadError = err
		close(s.chSocketReadError)
	})
}

// notifyWriteError records the first underlying-connection write error.
func (s *Session) notifyWriteError(err error) {
	s.socketWriteErrorOnce.Do(func() {
		s.socketWriteError = err
		close(s.chSocketWriteError)
	})
}

// notifyProtoError records the first protocol error.
func (s *Session) notifyProtoError(err error) {
	s.protoErrorOnce.Do(func() {
		s.protoError = err
		close(s.chProtoError)
	})
}

// IsClosed reports whether the session has shut down.
func (s *Session) IsClosed() bool {
	return s.closed.Load() != 0
}

// NumStreams returns the number of streams currently registered in the session.
func (s *Session) NumStreams() int {
	if s.IsClosed() {
		return 0
	}
	s.streamLock.Lock()
	defer s.streamLock.Unlock()
	return len(s.streams)
}

// SetDeadline sets a deadline used by Accept* calls.
// A zero time value disables the deadline.
func (s *Session) SetDeadline(t time.Time) error {
	s.acceptDeadline.Set(t)
	return nil
}

// LocalAddr returns the underlying connection's local address, if available.
func (s *Session) LocalAddr() net.Addr {
	if ts, ok := s.conn.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

// RemoteAddr returns the underlying connection's remote address, if available.
func (s *Session) RemoteAddr() net.Addr {
	if ts, ok := s.conn.(interface {
		RemoteAddr() net.Addr
	}); ok {
		return ts.RemoteAddr()
	}
	return nil
}

// streamClosed removes a closed stream and returns its unused receive tokens.
func (s *Session) streamClosed(sid uint32) {
	s.streamLock.Lock()
	defer s.streamLock.Unlock()

	stream, ok := s.streams[sid]
	if !ok {
		return
	}

	if n := stream.recycleTokens(); n > 0 {
		// Restore receive capacity held by unread data from this stream.
		if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
			s.notifyBucket()
		}
	}
	delete(s.streams, sid)
}

// returnTokens returns receive-buffer capacity after a stream reads data.
func (s *Session) returnTokens(n int) {
	if atomic.AddInt32(&s.bucket, int32(n)) > 0 {
		s.notifyBucket()
	}
}

// recvLoop reads and dispatches frames while receive-buffer capacity is available.
func (s *Session) recvLoop() {
	var hdr rawHeader
	var updHdr updHeader

	for {
		// Wait for receive capacity, or stop if the session closes.
		for atomic.LoadInt32(&s.bucket) <= 0 && !s.IsClosed() {
			select {
			case <-s.bucketNotify:
			case <-s.die:
				// With no receive capacity, shutdown wakes this wait directly.
				// A shutdown during io.ReadFull instead surfaces as a read error.
				return
			}
		}

		// Read the next frame header while receive capacity is available.
		_, err := io.ReadFull(s.conn, hdr[:])
		if err != nil {
			s.notifyReadError(err)
			return
		}

		// Record activity for keep-alive timeout detection.
		s.sessionIsActive.Store(1)

		// Reject frames from an incompatible protocol version.
		if hdr.Version() != byte(s.config.Version) {
			s.notifyProtoError(ErrInvalidProtocol)
			return
		}

		// Dispatch the frame according to its command.
		sid := hdr.StreamID()
		switch hdr.Cmd() {
		case cmdNOP:
			if hdr.Length() != 0 {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
		case cmdSYN: // Open a new peer-initiated stream.
			if hdr.Length() != 0 {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
			var accepted *stream
			s.streamLock.Lock()
			if !s.IsClosed() {
				if _, ok := s.streams[sid]; !ok {
					stream := newStream(sid, s.config.MaxFrameSize, s)
					s.streams[sid] = stream
					accepted = stream
				}
			}
			s.streamLock.Unlock()

			if accepted != nil {
				select {
				case s.chAccepts <- accepted:
				case <-s.die:
					return
				}
				if s.IsClosed() {
					s.drainAccepts()
					return
				}
			}

		case cmdFIN: // Mark the peer's write side as closed.
			if hdr.Length() != 0 {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
			s.streamLock.Lock()
			st := s.streams[sid]
			s.streamLock.Unlock()
			if st != nil {
				st.fin() // Wake stream operations waiting for peer data or closure.
			}

		case cmdPSH: // Read and queue stream payload data.
			if hdr.Length() == 0 {
				continue
			}

			// Read the complete payload before dispatching the frame.
			pNewbuf := defaultAllocator.Get(int(hdr.Length()))
			written, err := io.ReadFull(s.conn, *pNewbuf)
			if err != nil {
				s.notifyReadError(err)

				// Return the buffer because it cannot be queued.
				defaultAllocator.Put(pNewbuf)
				return
			}

			// Queue the payload on its target stream.
			s.streamLock.Lock()
			buffered := false
			if stream, ok := s.streams[sid]; ok {
				select {
				case <-stream.chFinEvent:
					// Ignore data sent after the peer closed its write side.
				default:
					stream.pushBytes(pNewbuf)
					// Tokens are charged only while payload is buffered; reads
					// and stream cleanup return them to the shared bucket.
					atomic.AddInt32(&s.bucket, -int32(written))
					stream.wakeupReader()
					buffered = true
				}
			}
			if !buffered {
				// Dropped payloads were never charged to the token bucket.
				defaultAllocator.Put(pNewbuf)
			}
			s.streamLock.Unlock()

		case cmdUPD: // Apply a protocol-version-2 flow-control update.
			if s.config.Version != 2 {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
			if hdr.Length() != szCmdUPD {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}

			_, err := io.ReadFull(s.conn, updHdr[:])
			if err != nil {
				s.notifyReadError(err)
				return
			}

			// Apply the update if the referenced stream is still registered.
			s.streamLock.Lock()
			st := s.streams[sid]
			s.streamLock.Unlock()
			if st != nil {
				st.update(updHdr.Consumed(), updHdr.Window())
			}

		default:
			s.notifyProtoError(ErrInvalidProtocol)
			return
		}
	}
}

// keepalive sends periodic NOP frames and closes an inactive session on timeout.
func (s *Session) keepalive() {
	tickerPing := time.NewTicker(s.config.KeepAliveInterval)
	tickerTimeout := time.NewTicker(s.config.KeepAliveTimeout)
	defer tickerPing.Stop()
	defer tickerTimeout.Stop()
	for {
		select {
		case <-tickerPing.C:
			writeFrameInternal(s, newFrame(byte(s.config.Version), cmdNOP, 0), tickerPing.C, CLSCTRL)
			s.notifyBucket() // Wake recvLoop in case it is waiting for capacity.
		case <-tickerTimeout.C:
			if !s.sessionIsActive.CompareAndSwap(1, 0) {
				// Do not time out while recvLoop is paused because the shared
				// receive buffer is full.
				if atomic.LoadInt32(&s.bucket) > 0 {
					s.Close()
					return
				}
			}
		case <-s.die:
			return
		}
	}
}

// shaperLoop prioritizes control frames and schedules data frames fairly across streams.
func (s *Session) shaperLoop() {
	defer func() {
		err := io.ErrClosedPipe
		select {
		case <-s.chSocketWriteError:
			if s.socketWriteError != nil {
				err = s.socketWriteError
			}
		default:
		}
		s.writeReqLock.Lock()
		s.discardPendingWrites(err)
		s.writeReqLock.Unlock()
		close(s.chShaperDone)
	}()
	chShaper := s.shaper

	for {
		select {
		case <-s.die:
			return
		case <-s.chSocketWriteError:
			return
		case r := <-chShaper:
			s.sq.Push(r)
			// Batch additional requests while there is room in the shaper queue.
			for len(chShaper) > 0 && s.sq.Len() < maxShaperSize {
				select {
				case r := <-chShaper:
					s.sq.Push(r)
				default:
				}
			}
			// Wake sendLoop to process the queued requests.
			s.notifyShaperPending()

			if s.sq.Len() >= maxShaperSize {
				// Pause admissions until sendLoop frees queue capacity.
				chShaper = nil
			}
		case <-s.chShaperConsumed:
			// Resume accepting requests after the queue has been consumed.
			chShaper = s.shaper
		}
	}
}

// discardPendingWrites completes queued requests with err during shutdown.
func (s *Session) discardPendingWrites(err error) {
	if err == nil {
		err = io.ErrClosedPipe
	}
	for {
		select {
		case req := <-s.shaper:
			select {
			case req.result <- writeResult{err: err}:
			default:
			}
		default:
			goto drainQueue
		}
	}

drainQueue:
	if s.sq == nil {
		return
	}
	for {
		req, ok := s.sq.Pop()
		if !ok {
			return
		}
		select {
		case req.result <- writeResult{err: err}:
		default:
		}
	}
}

// notifyShaperPending wakes sendLoop when requests are ready to be written.
func (s *Session) notifyShaperPending() {
	select {
	case s.chShaperPending <- struct{}{}:
	default:
	}
}

// notifyShaperConsumed allows shaperLoop to resume admitting requests.
func (s *Session) notifyShaperConsumed() {
	select {
	case s.chShaperConsumed <- struct{}{}:
	default:
	}
}

// sendLoop serializes queued frames onto the underlying connection.
func (s *Session) sendLoop() {
	var buf []byte
	var n int
	var err error
	var vec [][]byte // vector for writeBuffers

	bw, ok := s.conn.(interface {
		WriteBuffers(v [][]byte) (n int, err error)
	})

	if ok {
		buf = make([]byte, headerSize)
		vec = make([][]byte, 2)
	} else {
		buf = make([]byte, (1<<16)+headerSize)
	}

EVENT_LOOP:
	for {
		select {
		case <-s.die:
			return
		case <-s.chShaperPending:
			for {
				request, ok := s.sq.Pop()
				if !ok {
					// Let shaperLoop resume admissions now that the queue is empty.
					s.notifyShaperConsumed()
					goto EVENT_LOOP
				}

				buf[0] = request.frame.ver
				buf[1] = request.frame.cmd
				binary.LittleEndian.PutUint16(buf[2:], uint16(len(request.frame.data)))
				binary.LittleEndian.PutUint32(buf[4:], request.frame.sid)

				// Write the header and payload separately when supported.
				if len(vec) > 0 {
					vec[0] = buf[:headerSize]
					vec[1] = request.frame.data
					n, err = bw.WriteBuffers(vec)
				} else {
					copy(buf[headerSize:], request.frame.data)
					n, err = s.conn.Write(buf[:headerSize+len(request.frame.data)])
				}
				if err == nil && n != headerSize+len(request.frame.data) {
					err = io.ErrShortWrite
				}

				n -= headerSize
				if n < 0 {
					n = 0
				}

				result := writeResult{
					n:   n,
					err: err,
				}

				request.result <- result

				// Stop the writer after recording an underlying connection error.
				if err != nil {
					s.notifyWriteError(err)
					return
				}
			}
		}
	}
}

// writeControlFrame queues a control frame and waits up to openCloseTimeout
// for its write result.
func (s *Session) writeControlFrame(f Frame) (n int, err error) {
	timer := time.NewTimer(openCloseTimeout)
	defer timer.Stop()

	return writeFrameInternal(s, f, timer.C, CLSCTRL)
}

// sendWriteRequest queues a frame before deadline, session shutdown, or a
// connection write error.
func sendWriteRequest[T any](s *Session, req writeRequest, deadline <-chan T) error {
	s.writeReqLock.RLock()
	defer s.writeReqLock.RUnlock()

	if s.IsClosed() {
		return io.ErrClosedPipe
	}

	select {
	case <-s.chSocketWriteError:
		return s.socketWriteError
	default:
	}

	select {
	case s.shaper <- req:
		return nil
	case <-s.die:
		return io.ErrClosedPipe
	case <-s.chSocketWriteError:
		return s.socketWriteError
	case <-deadline:
		return ErrTimeout
	}
}

// writeFrameInternal queues a frame and waits for its write result or deadline.
func writeFrameInternal[T any](s *Session, f Frame, deadline <-chan T, class CLASSID) (n int, err error) {
	// Reuse a result channel to avoid allocating one for every frame.
	resultCh := resultChanPool.Get().(chan writeResult)

	err = sendWriteRequest(
		s,
		writeRequest{
			class:  class,
			frame:  f,
			seq:    s.requestID.Add(1),
			result: resultCh,
		},
		deadline,
	)
	if err != nil {
		goto recycle
	}

	select {
	case result := <-resultCh:
		n, err = result.n, result.err
	case <-s.die:
		// sendLoop may still deliver this request's result; keep the channel
		// out of the pool to prevent another request from reusing it.
		return 0, io.ErrClosedPipe
	case <-s.chSocketWriteError:
		// sendLoop may still deliver this request's result; keep the channel
		// out of the pool to prevent another request from reusing it.
		return 0, s.socketWriteError
	case <-deadline:
		// sendLoop may still deliver this request's result; keep the channel
		// out of the pool to prevent another request from reusing it.
		return 0, ErrTimeout
	}

recycle:
	resultChanPool.Put(resultCh)
	return
}
