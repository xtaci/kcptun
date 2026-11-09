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

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Temporary() bool { return true }
func (timeoutError) Timeout() bool   { return true }

var (
	ErrInvalidProtocol           = errors.New("invalid protocol")
	ErrConsumed                  = errors.New("peer consumed more than sent")
	ErrGoAway                    = errors.New("stream id overflows, should start a new connection")
	ErrTimeout         net.Error = &timeoutError{}
	ErrWouldBlock                = errors.New("operation would block on IO")
)

// writeRequest represents a request to write a frame
type writeRequest struct {
	class  CLASSID
	frame  Frame
	seq    uint32
	result chan writeResult
}

// writeResult represents the result of a write request
type writeResult struct {
	n   int
	err error
}

// Session defines a multiplexed connection for streams
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
	// Stop the Accept deadline timer when the listener is closed.
	runtime.AddCleanup(s, freeDeadline, s.acceptDeadline)

	go s.shaperLoop()
	go s.recvLoop()
	go s.sendLoop()
	if !config.KeepAliveDisabled {
		go s.keepalive()
	}
	return s
}

// generate stream id
func (s *Session) allocStreamID() (uint32, bool) {
	s.nextStreamIDLock.Lock()
	defer s.nextStreamIDLock.Unlock()

	if s.goAway > 0 {
		return 0, false
	}
	// check for stream id overflow
	if s.nextStreamID+2 < s.nextStreamID {
		s.goAway = 1
		return 0, false
	}
	// allocate next stream id
	s.nextStreamID += 2
	return s.nextStreamID, true
}

// OpenStream is used to create a new stream
func (s *Session) OpenStream() (*Stream, error) {
	if s.IsClosed() {
		return nil, io.ErrClosedPipe
	}

	sid, ok := s.allocStreamID()
	if !ok {
		return nil, ErrGoAway
	}

	stream := newStream(sid, s.config.MaxFrameSize, s)

	// register before sending SYN, so frames the peer sends right after
	// accepting are not dropped by recvLoop as belonging to an unknown stream.
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
		// NOTE(x): disabled finalizer for issue #997
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

// Open returns a generic ReadWriteCloser
func (s *Session) Open() (io.ReadWriteCloser, error) {
	return s.OpenStream()
}

// AcceptStream is used to block until the next available stream
// is ready to be accepted.
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
		// NOTE: no Close finalizer here (issue #116). io.Copy on an accepted
		// stream dispatches to WriteTo promoted from the embedded *stream, so
		// the outer *Stream wrapper becomes unreachable during the copy; a
		// finalizer would then close the stream mid-transfer and silently drop
		// data. OpenStream disabled the same finalizer for issue #997.
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

	// Prevent requests from being enqueued after shaperLoop drains the queues.
	s.writeReqLock.Lock()
	s.writeReqLock.Unlock()

	s.closeStreams()
	err := s.conn.Close()
	if s.chShaperDone != nil {
		<-s.chShaperDone
	}
	// A request admission that raced with the shutdown signal may have won
	// its select after shaperLoop's initial drain.
	s.discardPendingWrites(io.ErrClosedPipe)
	s.drainAccepts()
	return err
}

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

// notifyBucket notifies recvLoop that bucket is available
func (s *Session) notifyBucket() {
	select {
	case s.bucketNotify <- struct{}{}:
	default:
	}
}

func (s *Session) notifyReadError(err error) {
	s.socketReadErrorOnce.Do(func() {
		s.socketReadError = err
		close(s.chSocketReadError)
	})
}

func (s *Session) notifyWriteError(err error) {
	s.socketWriteErrorOnce.Do(func() {
		s.socketWriteError = err
		close(s.chSocketWriteError)
	})
}

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

// NumStreams returns the number of currently open streams
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

// LocalAddr satisfies net.Conn interface
func (s *Session) LocalAddr() net.Addr {
	if ts, ok := s.conn.(interface {
		LocalAddr() net.Addr
	}); ok {
		return ts.LocalAddr()
	}
	return nil
}

// RemoteAddr satisfies net.Conn interface
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
		// return remaining tokens to the bucket
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

// recvLoop reads frames from the underlying connection while receive tokens are available.
func (s *Session) recvLoop() {
	var hdr rawHeader
	var updHdr updHeader

	for {
		// Wait until we have tokens or session is closed.
		for atomic.LoadInt32(&s.bucket) <= 0 && !s.IsClosed() {
			select {
			case <-s.bucketNotify:
			case <-s.die:
				// If it returns here, Accept() and OpenStream() are unblocked with io.ErrClosedPipe,
				// causing recvLoop to exit gracefully. If recvLoop is blocked in io.ReadFull, however,
				// it will be unblocked by a socket read error instead.
				return
			}
		}

		// As long as we have tokens, try to read frames.
		// read header first
		_, err := io.ReadFull(s.conn, hdr[:])
		if err != nil {
			s.notifyReadError(err)
			return
		}

		// Mark the session as active
		s.sessionIsActive.Store(1)

		// validate protocol version
		if hdr.Version() != byte(s.config.Version) {
			s.notifyProtoError(ErrInvalidProtocol)
			return
		}

		// handle different command types
		sid := hdr.StreamID()
		switch hdr.Cmd() {
		case cmdNOP:
			if hdr.Length() != 0 {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
		case cmdSYN: // stream opening
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

		case cmdFIN: // stream closing
			if hdr.Length() != 0 {
				s.notifyProtoError(ErrInvalidProtocol)
				return
			}
			s.streamLock.Lock()
			st := s.streams[sid]
			s.streamLock.Unlock()
			if st != nil {
				st.fin() // fin unblocks the readers and writers
			}

		case cmdPSH: // data frame
			if hdr.Length() == 0 {
				continue
			}

			// read payload from the underlying connection
			pNewbuf := defaultAllocator.Get(int(hdr.Length()))
			written, err := io.ReadFull(s.conn, *pNewbuf)
			if err != nil {
				s.notifyReadError(err)

				// recycle the buffer immediately.
				defaultAllocator.Put(pNewbuf)
				return
			}

			// push data to the corresponding stream
			s.streamLock.Lock()
			if stream, ok := s.streams[sid]; ok {
				stream.pushBytes(pNewbuf)
				// deduct tokens from the bucket
				atomic.AddInt32(&s.bucket, -int32(written))
				stream.wakeupReader()
			} else {
				// data directed to a missing/closed stream, recycle the buffer immediately.
				defaultAllocator.Put(pNewbuf)
			}
			s.streamLock.Unlock()

		case cmdUPD: // a window update signal (v2 only)
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

			// update the window size for the corresponding stream
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

// keepalive sends NOP frames periodically to keep the connection alive
func (s *Session) keepalive() {
	tickerPing := time.NewTicker(s.config.KeepAliveInterval)
	tickerTimeout := time.NewTicker(s.config.KeepAliveTimeout)
	defer tickerPing.Stop()
	defer tickerTimeout.Stop()
	for {
		select {
		case <-tickerPing.C:
			writeFrameInternal(s, newFrame(byte(s.config.Version), cmdNOP, 0), tickerPing.C, CLSCTRL)
			s.notifyBucket() // force a wakeup signal to the recvLoop
		case <-tickerTimeout.C:
			if !s.sessionIsActive.CompareAndSwap(1, 0) {
				// recvLoop may block while bucket is 0, in this case,
				// session should not be closed.
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
			// batch drain: collect more requests if available
			for len(chShaper) > 0 && s.sq.Len() < maxShaperSize {
				select {
				case r := <-chShaper:
					s.sq.Push(r)
				default:
				}
			}
			// notify sendLoop there are pending requests
			s.notifyShaperPending()

			if s.sq.Len() >= maxShaperSize {
				// stop accepting new requests temporarily if shaper queue is full
				chShaper = nil
			}
		case <-s.chShaperConsumed:
			// re-enable shaper channel
			chShaper = s.shaper
		}
	}
}

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

// notifyShaperPending notifies sendLoop that there are pending requests
func (s *Session) notifyShaperPending() {
	select {
	case s.chShaperPending <- struct{}{}:
	default:
	}
}

// notifyShaperConsumed notifies when shaper queue is being consumed
func (s *Session) notifyShaperConsumed() {
	select {
	case s.chShaperConsumed <- struct{}{}:
	default:
	}
}

// sendLoop sends frames over the underlying connection
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
					// notify shaperLoop to accept new requests
					s.notifyShaperConsumed()
					goto EVENT_LOOP
				}

				buf[0] = request.frame.ver
				buf[1] = request.frame.cmd
				binary.LittleEndian.PutUint16(buf[2:], uint16(len(request.frame.data)))
				binary.LittleEndian.PutUint32(buf[4:], request.frame.sid)

				// support for scatter-gather I/O
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

				// store conn error
				if err != nil {
					s.notifyWriteError(err)
					return
				}
			}
		}
	}
}

// writeControlFrame writes the control frame to the underlying connection
// and returns the number of bytes written if successful
func (s *Session) writeControlFrame(f Frame) (n int, err error) {
	timer := time.NewTimer(openCloseTimeout)
	defer timer.Stop()

	return writeFrameInternal(s, f, timer.C, CLSCTRL)
}

// TODO: generic methods requires Go 1.27 or later, but we want to support Go 1.24 or later.
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

// internal writeFrame version to support deadline used in keepalive
func writeFrameInternal[T any](s *Session, f Frame, deadline <-chan T, class CLASSID) (n int, err error) {
	// get result channel from pool
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
		// Cannot recycle channel here - sendLoop may still write to it
		return 0, io.ErrClosedPipe
	case <-s.chSocketWriteError:
		// Cannot recycle channel here - sendLoop may still write to it
		return 0, s.socketWriteError
	case <-deadline:
		// Cannot recycle channel here - sendLoop may still write to it
		return 0, ErrTimeout
	}

recycle:
	resultChanPool.Put(resultCh)
	return
}
