// The MIT License (MIT)
//
// Copyright (c) 2015 xtaci
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

// [THE GENERALIZED DATA PIPELINE FOR KCP-GO]
//
// Outgoing Data Pipeline:                        Incoming Data Pipeline:
// Stream          (Input Data)                   Packet Network  (Network Interface Card)
//   |                                               |
//   v                                               v
// KCP Output      (Reliable Transport Layer)     Reader/Listener (Reception Queue)
//   |                                               |
//   v                                               v
// FEC Encoding    (Forward Error Correction)     Decryption/authentication
//   |                                               |
//   v                                               v
// CFB: CRC32      (Error Detection)              CFB: CRC32 verification
// AEAD: authentication                           AEAD: authentication
//   |                                               |
//   v                                               v
// Encryption      (CFB or AEAD)                  FEC Decoding    (Forward Error Correction)
//   |                                               |
//   v                                               v
// TxQueue         (Transmission Queue)           KCP Input       (Reliable Transport Layer)
//   |                                               |
//   v                                               v
// Packet Network  (Network Transmission)         Stream          (Input Data)

package kcp

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"hash/crc32"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/ratelimit"
	"github.com/pion/transport/v5/deadline"
	"github.com/pkg/errors"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// Session-layer constants
const (
	// 16-byte nonce for each packet using a legacy block cipher
	nonceSize = 16

	// 4-byte CRC32 checksum per packet using a legacy block cipher
	crcSize = 4

	// legacy block-cipher header size: nonce + CRC32
	cryptHeaderSize = nonceSize + crcSize

	// maximum packet size (IPv4)
	maxPacketSize = 1472

	// conversation ID field size (bytes)
	convSize = 4

	// accept backlog: max pending connections for Listener
	acceptBacklog = 128

	// devBacklog: maximum queued requests in the post-processing pipeline
	devBacklog = 2048

	// Maximum interval between consecutive FEC data packets (ms) for parity
	// generation. If the interval meets or exceeds this, parity is skipped.
	maxFECEncodeLatency = 500

	// max number of packets batched in a single sendmmsg/writev call
	maxBatchSize = 64
)

var (
	errInvalidOperation = errors.New("invalid operation")
	errTimeout          = timeoutError{}
	errNotOwner         = errors.New("not the owner of this connection")
)

// timeoutError implements net.Error
type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// sendRequest holds a write request before encoding and transmission.
type sendRequest struct {
	buffer []byte
	oob    bool
}

// OOB callback function
type OOBCallBackType func([]byte)

type (
	// UDPSession is a KCP session over UDP.
	UDPSession struct {
		conn    net.PacketConn // the underlying packet connection
		ownConn bool           // true if we created conn internally, false if provided by caller
		kcp     *KCP           // KCP ARQ protocol
		l       *Listener      // pointing to the Listener object if it's been accepted by a Listener
		block   BlockCrypt     // block encryption object

		updateFn func() // s.update, made once so rescheduling does not allocate

		// kcp receiving is based on packets
		// recvbuf turns packets into stream
		recvbuf []byte
		input   bytes.Reader

		// FEC codec
		fecDecoder *fecDecoder
		fecEncoder *fecEncoder

		// settings
		remote                      net.Addr // remote peer address
		pushedFrom                  net.Addr // peer pinned by pushedPacket when remote is nil
		readDeadline, writeDeadline *deadline.Deadline
		readNotify                  atomic.Pointer[func()] // see SetReadNotify
		headerSize                  int                    // the header size additional to a KCP frame
		ackNoDelay                  bool                   // send ACKs immediately for each incoming packet (testing only)
		writeDelay                  bool                   // delay kcp.flush() for Write() for bulk transfer
		dup                         int                    // duplicate UDP packets (testing only)

		// notifications
		die          chan struct{} // notify current session has Closed
		dieOnce      sync.Once
		chReadEvent  chan struct{} // notify Read() can be called without blocking
		chWriteEvent chan struct{} // notify Write() can be called without blocking

		// socket error handling
		socketReadError      atomic.Value
		socketWriteError     atomic.Value
		chSocketReadError    chan struct{}
		chSocketWriteError   chan struct{}
		socketReadErrorOnce  sync.Once
		socketWriteErrorOnce sync.Once

		// packets waiting to be sent on wire
		postQueue sendQueue
		ppRunning atomic.Bool // queued for or served by a postProcess worker
		ppSendMu  sync.Mutex
		ppClosing bool

		// platform-dependent optimizations
		platform platform

		// ratelimit buckets
		bucket atomic.Pointer[ratelimit.Bucket]

		writeGate chan struct{}
		mu        sync.Mutex

		// callbackForOOB is an optional callback for handling received out-of-band (OOB) data.
		//
		// OOB data bypasses the KCP reliable data path and is delivered unreliably.
		// The callback is invoked synchronously from the KCP input processing path.
		callbackForOOB atomic.Value
	}

	setReadBuffer interface {
		SetReadBuffer(bytes int) error
	}

	setWriteBuffer interface {
		SetWriteBuffer(bytes int) error
	}

	setDSCP interface {
		SetDSCP(int) error
	}
)

// newUDPSession creates a new UDP session for a client or server.
func newUDPSession(conv uint32, dataShards, parityShards int, l *Listener, conn net.PacketConn, ownConn bool, remote net.Addr, block BlockCrypt) *UDPSession {
	sess := new(UDPSession)
	sess.writeGate = make(chan struct{}, 1)
	sess.writeGate <- struct{}{}
	sess.die = make(chan struct{})
	sess.chReadEvent = make(chan struct{}, 1)
	sess.chWriteEvent = make(chan struct{}, 1)
	sess.chSocketReadError = make(chan struct{})
	sess.chSocketWriteError = make(chan struct{})
	sess.remote = remote
	sess.conn = conn
	sess.ownConn = ownConn
	sess.l = l
	sess.block = block
	sess.recvbuf = make([]byte, maxPacketSize)
	sess.initPlatform()

	sess.readDeadline = deadline.New()
	sess.writeDeadline = deadline.New()

	// Stop pending timers so closed sessions do not retain deadline resources
	// until their deadlines expire.
	runtime.AddCleanup(sess, freeDeadline, sess.readDeadline)
	runtime.AddCleanup(sess, freeDeadline, sess.writeDeadline)

	// calculate additional header size introduced by encryption
	switch block := sess.block.(type) {
	case nil:
		sess.headerSize = 0
	case *aeadCrypt:
		sess.headerSize = block.NonceSize()
	default:
		sess.headerSize = cryptHeaderSize
	}

	// FEC codec initialization
	sess.fecDecoder = newFECDecoder(dataShards, parityShards)
	sess.fecEncoder = newFECEncoder(dataShards, parityShards, sess.headerSize)

	// calculate additional header size introduced by FEC
	if sess.fecEncoder != nil {
		sess.headerSize += fecHeaderSizePlus2
	}

	sess.kcp = NewKCP(conv, func(buf []byte, size int) {
		// A basic check for the minimum packet size
		if size >= IKCP_OVERHEAD {
			// make a copy
			bts := defaultBufferPool.Get()[:size+sess.headerSize]
			// copy the data to a new buffer, and reserve header space
			copy(bts[sess.headerSize:], buf)

			// delivery to post processing (non-blocking to avoid deadlock under lock)
			select {
			case <-sess.die:
				defaultBufferPool.Put(bts)
				return
			default:
			}
			if sess.postQueue.push(sendRequest{bts, false}) {
				sess.kickPostProcess()
			} else {
				// drop and recycle to avoid blocking; KCP will retransmit if needed
				defaultBufferPool.Put(bts)
			}
		}
	})

	// Set Default MTU
	if !sess.SetMtu(IKCP_MTU_DEF) {
		panic("Overhead too large")
	}

	if sess.l == nil { // it's a client connection
		if pp, ok := conn.(PacketPusher); ok {
			pp.SetPacketReceiver(sess.pushedPacket)
		} else {
			go sess.readLoop()
		}
		DefaultSnmp.increaseActiveOpens()
	} else {
		DefaultSnmp.increasePassiveOpens()
	}

	// start per-session updater
	sess.updateFn = sess.update
	SystemTimedSched.Put(sess.updateFn, time.Now())

	DefaultSnmp.increaseCurrEstab()

	return sess
}

// readLocked copies data that is ready into b and reports whether there was
// any. It must be called with s.mu held.
func (s *UDPSession) readLocked(b []byte) (int, bool) {
	// bufptr points to the current position in recvbuf. If the previous read's
	// buffer was too small, the remaining data is saved in bufptr for the next
	// read.
	if s.input.Len() > 0 {
		n, _ := s.input.Read(b)
		s.notifyReadIfAvailableLocked()
		return n, true
	}

	size := s.kcp.PeekSize() // peek data size from kcp
	if size <= 0 {
		return 0, false
	}
	// If b is large enough, read directly from KCP into b without an
	// intermediate buffer.
	if len(b) >= size {
		s.kcp.Recv(b)
		s.notifyReadIfAvailableLocked()
		return size, true
	}

	// Otherwise, read into recvbuf first, then copy to b. Grow recvbuf to fit
	// the packet when necessary.
	if cap(s.recvbuf) < size {
		// recvbuf is usually sized to the maximum packet size
		s.recvbuf = make([]byte, size)
	}

	// resize the length of recvbuf to match the data size
	s.recvbuf = s.recvbuf[:size]
	s.kcp.Recv(s.recvbuf)    // read data to recvbuf first
	s.input.Reset(s.recvbuf) // pointer update

	n, _ := s.input.Read(b) // copy to b
	s.notifyReadIfAvailableLocked()
	return n, true
}

func (s *UDPSession) read(b []byte) (n int, ok bool) {
	s.mu.Lock()
	n, ok = s.readLocked(b)
	s.mu.Unlock()

	if !ok {
		return
	}

	DefaultSnmp.addBytesReceived(n)
	return
}

// Read implements net.Conn
func (s *UDPSession) Read(b []byte) (n int, err error) {
	for {
		n, ok := s.read(b)
		if ok {
			return n, nil
		}

		select {
		case <-s.chReadEvent:
		case <-s.readDeadline.Done():
			return 0, errors.WithStack(errTimeout)
		case <-s.chSocketReadError:
			return 0, s.socketReadError.Load().(error)
		case <-s.die:
			return 0, errors.WithStack(io.ErrClosedPipe)
		}
	}
}

func (s *UDPSession) writeBuffersLocked(v [][]byte) (int, bool) {
	// make sure write do not overflow the max sliding window on both side
	waitsnd := s.kcp.WaitSnd()
	if waitsnd >= int(s.kcp.snd_wnd) {
		return 0, false
	}

	var n int
	// Transmit data sequentially, keeping each packet within mss.
	for _, b := range v {
		n += len(b)
		// handle each slice for packet splitting
		for {
			if len(b) <= int(s.kcp.mss) {
				s.kcp.Send(b)
				break
			}

			s.kcp.Send(b[:s.kcp.mss])
			b = b[s.kcp.mss:]
		}
	}

	waitsnd = s.kcp.WaitSnd()
	if waitsnd >= int(s.kcp.snd_wnd) || !s.writeDelay {
		// put the packets on wire immediately if the inflight window is full
		// or if write delay is disabled (no merging of outgoing bytes),
		// so we do not have to wait for the periodic update to uncork.
		s.kcp.flush(IKCP_FLUSH_FULL)
	}
	return n, true
}

func (s *UDPSession) writeBuffers(v [][]byte) (n int, ok bool) {
	s.mu.Lock()
	n, ok = s.writeBuffersLocked(v)
	s.mu.Unlock()

	if !ok {
		return
	}

	DefaultSnmp.addBytesSent(n)
	s.writeWait(n)
	return
}

// WriteBuffers writes a vector of byte slices to the underlying connection.
func (s *UDPSession) WriteBuffers(v [][]byte) (n int, err error) {
	// An empty write only reports the conn's state. It used to run a full
	// flush, which callers that probe with Write(nil) paid on every frame.
	if buffersEmpty(v) {
		select {
		case <-s.chSocketWriteError:
			return 0, s.socketWriteError.Load().(error)
		case <-s.die:
			return 0, errors.WithStack(io.ErrClosedPipe)
		default:
			return 0, nil
		}
	}
	err = s.acquireWriteGate()
	if err != nil {
		return 0, err
	}
	defer s.releaseWriteGate()

	for {
		// check for connection close, socket error and deadline
		select {
		case <-s.writeDeadline.Done():
			return 0, errors.WithStack(errTimeout)
		case <-s.chSocketWriteError:
			return 0, s.socketWriteError.Load().(error)
		case <-s.die:
			return 0, errors.WithStack(io.ErrClosedPipe)
		default:
		}

		n, ok := s.writeBuffers(v)
		if ok {
			return n, nil
		}

		select {
		case <-s.chWriteEvent:
		case <-s.writeDeadline.Done():
			return 0, errors.WithStack(errTimeout)
		case <-s.chSocketWriteError:
			return 0, s.socketWriteError.Load().(error)
		case <-s.die:
			return 0, errors.WithStack(io.ErrClosedPipe)
		}
	}
}

// Write implements net.Conn
func (s *UDPSession) Write(b []byte) (n int, err error) { return s.WriteBuffers([][]byte{b}) }

// writeWait paces outgoing bytes against the configured rate bucket.
// The data is already in KCP and accounted for when this runs; only the
// wait is left.
func (s *UDPSession) writeWait(n int) {
	bucket := s.bucket.Load()
	if bucket == nil {
		return
	}

	bucket.Wait(int64(n))
}

// acquireWriteGate serializes non-empty writes while allowing a blocked caller
// to observe its deadline, socket errors, and session closure.
func (s *UDPSession) acquireWriteGate() error {
	select {
	case <-s.writeGate:
		return nil
	default:
	}
	return s.waitWriteGate()
}

// waitWriteGate handles the contended gate path, including deadline updates
// signaled through chWriteEvent.
func (s *UDPSession) waitWriteGate() error {
	for {
		select {
		case <-s.chWriteEvent:
		case <-s.writeGate:
			return nil
		case <-s.writeDeadline.Done():
			return errors.WithStack(errTimeout)
		case <-s.chSocketWriteError:
			return s.socketWriteError.Load().(error)
		case <-s.die:
			return errors.WithStack(io.ErrClosedPipe)
		}
	}
}

// releaseWriteGate makes the session available to the next waiting writer.
func (s *UDPSession) releaseWriteGate() {
	s.writeGate <- struct{}{}
}

func (s *UDPSession) isClosed() bool {
	select {
	case <-s.die:
		return true
	default:
		return false
	}
}

// Close closes the connection.
func (s *UDPSession) Close() error {
	var retErr error
	var once bool
	s.dieOnce.Do(func() {
		once = true

		// try best to send all queued messages especially the data in txqueue
		s.mu.Lock()
		s.kcp.flush((IKCP_FLUSH_FULL))
		close(s.die)
		s.mu.Unlock()

		s.ppSendMu.Lock()
		s.ppClosing = true
		s.ppSendMu.Unlock()

		DefaultSnmp.decreaseCurrEstab()
		s.callReadNotify()
		s.discardPostProcess()

		if s.l != nil { // belongs to listener
			s.l.closeSession(s.remote)
			return
		}

		if pp, ok := s.conn.(PacketPusher); ok {
			pp.SetPacketReceiver(nil)
		}

		if s.ownConn { // client socket close
			retErr = s.conn.Close()
		}
	})
	if !once {
		return errors.WithStack(io.ErrClosedPipe)
	}
	return retErr
}

// LocalAddr returns the local network address. The Addr returned is shared by all invocations of LocalAddr, so do not modify it.
func (s *UDPSession) LocalAddr() net.Addr { return s.conn.LocalAddr() }

// RemoteAddr returns the remote network address. The Addr returned is shared by all invocations of RemoteAddr, so do not modify it.
func (s *UDPSession) RemoteAddr() net.Addr { return s.remote }

// SetDeadline sets the session's read and write deadlines. A zero time value
// disables the deadlines.
func (s *UDPSession) SetDeadline(t time.Time) error {
	s.SetReadDeadline(t)
	s.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline implements the Conn SetReadDeadline method.
func (s *UDPSession) SetReadDeadline(t time.Time) error {
	s.readDeadline.Set(t)
	return nil
}

// SetWriteDeadline implements the Conn SetWriteDeadline method.
func (s *UDPSession) SetWriteDeadline(t time.Time) error {
	s.writeDeadline.Set(t)
	return nil
}

// SetWriteDelay delays writes during bulk transfer until the next update interval.
func (s *UDPSession) SetWriteDelay(delay bool) {
	s.mu.Lock()
	s.writeDelay = delay
	s.mu.Unlock()
}

// SetWindowSize sets the maximum window sizes.
func (s *UDPSession) SetWindowSize(sndwnd, rcvwnd int) {
	s.mu.Lock()
	s.kcp.WndSize(sndwnd, rcvwnd)
	s.mu.Unlock()
}

// SetMtu sets the maximum transmission unit, excluding the UDP header.
func (s *UDPSession) SetMtu(mtu int) bool {
	mtu = min(maxPacketSize, mtu)

	mtu -= s.headerSize
	if aead, ok := s.block.(*aeadCrypt); ok {
		mtu -= aead.Overhead()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	ret := s.kcp.SetMtu(mtu) // kcp mtu is not including udp header
	return ret == 0
}

// Deprecated: toggles the stream mode on/off
func (s *UDPSession) SetStreamMode(enable bool) {
	s.mu.Lock()
	if enable {
		s.kcp.stream = 1
	} else {
		s.kcp.stream = 0
	}
	s.mu.Unlock()
}

// SetACKNoDelay controls ACK flushing. When true, ACKs are flushed immediately.
func (s *UDPSession) SetACKNoDelay(nodelay bool) {
	s.mu.Lock()
	s.ackNoDelay = nodelay
	s.mu.Unlock()
}

// (deprecated)
//
// SetDUP duplicates UDP packets sent by KCP.
func (s *UDPSession) SetDUP(dup int) {
	s.mu.Lock()
	s.dup = dup
	s.mu.Unlock()
}

// SetNoDelay sets the KCP no-delay options.
// https://github.com/skywind3000/kcp/blob/master/README.en.md#protocol-configuration
func (s *UDPSession) SetNoDelay(nodelay, interval, resend, nc int) {
	s.mu.Lock()
	s.kcp.NoDelay(nodelay, interval, resend, nc)
	s.mu.Unlock()
}

// SetDSCP sets the 6-bit DSCP field in the IPv4 header or the 6-bit Traffic
// Class field in the IPv6 header.
//
// If the underlying connection implements `SetDSCP(int) error`, SetDSCP invokes
// that method instead.
//
// It has no effect if it's accepted from Listener.
func (s *UDPSession) SetDSCP(dscp int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l != nil {
		return errInvalidOperation
	}

	// interface enabled
	if ts, ok := s.conn.(setDSCP); ok {
		return ts.SetDSCP(dscp)
	}

	if nc, ok := s.conn.(net.Conn); ok {
		val := dscp << 2

		var succeed bool
		if err := ipv4.NewConn(nc).SetTOS(val); err == nil {
			succeed = true
		}
		if err := ipv6.NewConn(nc).SetTrafficClass(val); err == nil {
			succeed = true
		}

		if succeed {
			return nil
		}
	}
	return errInvalidOperation
}

// SetReadBuffer sets the socket read buffer, no effect if it's accepted from Listener
func (s *UDPSession) SetReadBuffer(bytes int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l == nil {
		if nc, ok := s.conn.(setReadBuffer); ok {
			return nc.SetReadBuffer(bytes)
		}
	}
	return errInvalidOperation
}

// SetWriteBuffer sets the socket write buffer, no effect if it's accepted from Listener
func (s *UDPSession) SetWriteBuffer(bytes int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.l == nil {
		if nc, ok := s.conn.(setWriteBuffer); ok {
			return nc.SetWriteBuffer(bytes)
		}
	}
	return errInvalidOperation
}

// SetRateLimit limits the rate at which Write and WriteBuffers accept
// application bytes. Protocol overhead, retransmissions, and SendOOB traffic
// are not counted. A value of 0 disables rate limiting.
func (s *UDPSession) SetRateLimit(bps uint32) {
	var bucket *ratelimit.Bucket
	if bps > 0 {
		bucket = ratelimit.NewBucketWithRate(float64(bps), int64(bps))
	}

	s.bucket.Store(bucket)
}

// SetBucket limits the rate at which Write and WriteBuffers accept
// application bytes. Protocol overhead, retransmissions, and SendOOB traffic
// are not counted. A value of nil disables rate limiting.
func (s *UDPSession) SetBucket(bucket *ratelimit.Bucket) {
	s.bucket.Store(bucket)
}

// SetLogger configures the kcp trace logger
func (s *UDPSession) SetLogger(mask KCPLogType, logger logoutput_callback) {
	s.mu.Lock()
	s.kcp.SetLogger(mask, logger)
	s.mu.Unlock()
}

// Control applies a function to the underlying packet connection.
// CAUTION: BE VERY CAREFUL TO USE THIS FUNCTION, YOU MAY BREAK THE PROTOCOL.
func (s *UDPSession) Control(f func(conn net.PacketConn) error) error {
	if !s.ownConn {
		return errNotOwner
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return f(s.conn)
}

// postProcess runs the outgoing packet pipeline for s on a shared worker.
// It runs the following stages sequentially for each packet:
//  1. FEC encoding   — generate parity shards (Reed-Solomon)
//  2. Encryption     — AEAD (e.g. AES-GCM) or CFB mode with CRC32
//  3. TX batching    — accumulate packets and flush via sendmmsg/writev
//
// Pipeline: KCP output -> postQueue -> [FEC] -> [Encrypt] -> TxQueue -> Network
//
// It returns once the queue is empty or after ppBudget packets.
func (s *UDPSession) postProcess(w *ppWorker) (more bool) {
	bytesToSend := 0
	for range ppBudget {
		req, ok := s.postQueue.pop()
		if !ok {
			return false
		}
		buf := req.buffer
		oob := req.oob

		var ecc [][]byte

		// --- Stage 1: FEC encoding ---
		if s.fecEncoder != nil {
			if !oob {
				ecc = s.fecEncoder.encode(buf, maxFECEncodeLatency)
			} else {
				s.fecEncoder.encodeOOB(buf)
			}
		}

		// --- Stage 2: Encryption ---
		// Two modes supported:
		//   - AEAD (e.g. AES-GCM): nonce + authenticated ciphertext, no separate CRC
		//   - CFB (legacy block ciphers): random nonce + CRC32 checksum + CFB encryption
		switch block := s.block.(type) {
		case nil:
		case *aeadCrypt: // AEAD mode
			nonceSize := block.NonceSize()

			dst := buf[:nonceSize]
			nonce := buf[:nonceSize]
			plaintext := buf[nonceSize:]

			fillRand(nonce)
			buf = block.Seal(dst, nonce, plaintext, nil)

			for k := range ecc {
				dst := ecc[k][:nonceSize]
				nonce := ecc[k][:nonceSize]
				plaintext := ecc[k][nonceSize:]

				fillRand(nonce)
				ecc[k] = block.Seal(dst, nonce, plaintext, nil)
			}
		default: // Cipher Feedback (CFB) mode
			fillRand(buf[:nonceSize])
			checksum := crc32.ChecksumIEEE(buf[cryptHeaderSize:])
			binary.LittleEndian.PutUint32(buf[nonceSize:], checksum)
			block.Encrypt(buf, buf)

			for k := range ecc {
				fillRand(ecc[k][:nonceSize])
				checksum := crc32.ChecksumIEEE(ecc[k][cryptHeaderSize:])
				binary.LittleEndian.PutUint32(ecc[k][nonceSize:], checksum)
				block.Encrypt(ecc[k], ecc[k])
			}
		}

		// --- Stage 3: TX batching ---
		var msg ipv4.Message
		msg.Addr = s.remote

		// original copy, move buf to txqueue directly
		msg.Buffers = w.oneBuf(buf)
		bytesToSend += len(buf)
		w.txqueue = append(w.txqueue, msg)

		// dup copies for testing if set
		for i := 0; i < s.dup; i++ {
			bts := defaultBufferPool.Get()[:len(buf)]
			copy(bts, buf)
			msg.Buffers = w.oneBuf(bts)
			bytesToSend += len(bts)
			w.txqueue = append(w.txqueue, msg)
		}

		// parity
		for k := range ecc {
			bts := defaultBufferPool.Get()[:len(ecc[k])]
			copy(bts, ecc[k])
			msg.Buffers = w.oneBuf(bts)
			bytesToSend += len(bts)
			w.txqueue = append(w.txqueue, msg)
		}

		// transmit when postQueue is empty or we've reached max batch size
		if s.postQueue.len() == 0 || len(w.txqueue) >= maxBatchSize {
			if s.flushTx(w, bytesToSend) {
				return true
			}
			bytesToSend = 0
		}
	}
	if len(w.txqueue) > 0 && s.flushTx(w, bytesToSend) {
		return true
	}
	return s.postQueue.len() > 0
}

// update advances the KCP protocol state.
func (s *UDPSession) update() {
	select {
	case <-s.die:
	default:
		s.mu.Lock()
		interval := s.kcp.flush(IKCP_FLUSH_FULL)
		waitsnd := s.kcp.WaitSnd()
		if waitsnd < int(s.kcp.snd_wnd) {
			s.notifyWriteEvent()
		}
		s.mu.Unlock()
		// self-synchronized timed scheduling
		fn := s.updateFn
		if fn == nil {
			fn = s.update
		}
		SystemTimedSched.Put(fn, time.Now().Add(time.Duration(interval)*time.Millisecond))
	}
}

// GetConv returns the session's conversation ID.
func (s *UDPSession) GetConv() uint32 { return s.kcp.conv }

// GetRTO returns the session's current RTO.
func (s *UDPSession) GetRTO() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kcp.rx_rto
}

// GetSRTT returns the session's current smoothed RTT.
func (s *UDPSession) GetSRTT() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kcp.rx_srtt
}

// GetSRTTVar returns the session's current RTT variance.
func (s *UDPSession) GetSRTTVar() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.kcp.rx_rttvar
}

// SetOOBHandler registers a callback for receiving out-of-band (OOB) data.
//
// OOB data is delivered unreliably and bypasses the KCP reliable data path.
// The callback is invoked synchronously from the KCP input processing path.
//
// The callback MUST return quickly and MUST NOT perform any blocking operations.
// Blocking inside the callback will stall processing of all other KCP packets.
//
// Passing a nil callback unregisters the current OOB callback.
//
// OOB support requires FEC to be enabled, as the OOB packet format
// reuses the FEC header layout for demultiplexing.
func (s *UDPSession) SetOOBHandler(callback OOBCallBackType) error {
	if s.fecEncoder == nil {
		return errors.New("OOB requires FEC to be enabled")
	}
	if callback == nil {
		s.callbackForOOB.Store(OOBCallBackType(func([]byte) {}))
		return nil
	}
	s.callbackForOOB.Store(callback)
	return nil
}

// GetOOBMaxSize returns the maximum payload size for an OOB packet.
//
// The returned value is the maximum number of bytes that can be carried as
// OOB data in a single packet, based on the current MTU and protocol layout.
//
// If FEC is not enabled, OOB is unsupported and this function returns 0.
func (s *UDPSession) GetOOBMaxSize() int {
	if s.fecEncoder == nil {
		return 0
	}
	// Packet layout: | conv (4B) | OOB payload |
	s.mu.Lock()
	size := int(s.kcp.mtu) - convSize
	s.mu.Unlock()
	return size
}

// SendOOB sends an out-of-band (OOB) data packet.
//
// OOB packets:
//   - Are unreliable: they are NOT retransmitted if lost.
//   - Are unordered: delivery order is not guaranteed.
//   - Are unacknowledged: no ACKs are generated.
//   - Bypass the KCP reliable data path.
//   - Reuse the FEC header layout for demultiplexing, but are NOT protected by FEC.
//
// The OOB payload MUST fit into a single packet.
// If the payload is too large, an error is returned.
//
// If the internal send queue is full, the OOB packet is dropped silently.
func (s *UDPSession) SendOOB(data []byte) error {
	if s.fecEncoder == nil {
		return errors.New("OOB requires FEC to be enabled")
	}

	// lock the session during OOB packet construction
	s.mu.Lock()
	defer s.mu.Unlock()

	// Packet layout: | conv (4B) | OOB payload |
	size := convSize + len(data)
	if size > int(s.kcp.mtu) {
		return errors.New("OOB payload too large")
	}

	// Allocate buffer with reserved header space.
	// s.headerSize includes the space needed by the FEC encoder.
	buf := defaultBufferPool.Get()[:size+s.headerSize]
	// Encode conversation ID.
	binary.LittleEndian.PutUint32(buf[s.headerSize:], s.kcp.conv)

	// Copy OOB payload immediately after the conversation ID.
	copy(buf[s.headerSize+convSize:], data)

	// Enqueue the packet for post-processing.
	// Performs OOB framing, encryption, and transmission, bypassing FEC and KCP.
	select {
	case <-s.die:
		// Session is closing.
		defaultBufferPool.Put(buf)
		return errors.WithStack(io.ErrClosedPipe)
	default:
	}
	if s.postQueue.push(sendRequest{buf, true}) {
		s.kickPostProcess()
	} else {
		// Drop silently to avoid blocking the sender.
		// OOB delivery is best-effort by design.
		defaultBufferPool.Put(buf)
	}
	return nil
}

func (s *UDPSession) notifyReadEvent() {
	select {
	case s.chReadEvent <- struct{}{}:
	default:
	}
	s.callReadNotify()
}

func (s *UDPSession) notifyWriteEvent() {
	select {
	case s.chWriteEvent <- struct{}{}:
	default:
	}
}

func (s *UDPSession) notifyReadError(err error) {
	s.socketReadErrorOnce.Do(func() {
		s.socketReadError.Store(err)
		close(s.chSocketReadError)
	})
	s.callReadNotify()
}

func (s *UDPSession) notifyWriteError(err error) {
	s.socketWriteErrorOnce.Do(func() {
		s.socketWriteError.Store(err)
		close(s.chSocketWriteError)
	})
}

// -----------------------------------------------------------------------
// Packet input pipeline (decryption -> integrity check -> FEC -> KCP)
// -----------------------------------------------------------------------

// packetInput is the entry point for incoming packets.
// It handles decryption and integrity verification before passing data to
// kcpInput. CFB mode uses CRC32; AEAD mode verifies its authentication tag.
//
// Pipeline: Network -> [Decrypt and verify] -> kcpInput
func (s *UDPSession) packetInput(data []byte) {
	switch block := s.block.(type) {
	case nil:
	case *aeadCrypt:
		nonceSize := block.NonceSize()
		if len(data) < nonceSize+block.Overhead() {
			return
		}

		nonce := data[:nonceSize]
		ciphertext := data[nonceSize:]

		plaintext, err := block.Open(ciphertext[:0], nonce, ciphertext, nil)
		if err != nil {
			DefaultSnmp.increaseInCsumErrors()
			return
		}

		data = plaintext
	default:
		// decryption and crc32 check
		if len(data) < cryptHeaderSize {
			return
		}

		block.Decrypt(data, data)
		data = data[nonceSize:]

		checksum := crc32.ChecksumIEEE(data[crcSize:])
		if checksum != binary.LittleEndian.Uint32(data) {
			DefaultSnmp.increaseInCsumErrors()
			return
		}

		data = data[crcSize:]
	}

	// basic check for minimum packet size
	// NOTE: OOB allows sending small packets and even empty packets.
	if len(data) < min(IKCP_OVERHEAD, fecHeaderSizePlus2+convSize) {
		DefaultSnmp.addKCPInErrors(1)
		return
	}

	s.kcpInput(data)
}

// kcpInput routes a decrypted packet into the KCP state machine,
// handling FEC decoding and OOB delivery.
//
// Packet demultiplexing uses the 16-bit field at offset 4:
//   - 0xf1 (typeData) / 0xf2 (typeParity): FEC-encoded packet
//   - 0xf3 (typeOOB): out-of-band packet (unreliable, bypasses KCP)
//   - other values: raw KCP packet (no FEC)
//
// Note: KCP cmd values [81-84] with frg [0-255] do not collide with
// FEC type markers 0x00f1/0x00f2/0x00f3 in little-endian.
func (s *UDPSession) kcpInput(data []byte) {
	DefaultSnmp.increaseInPkts()
	DefaultSnmp.addInBytes(len(data))

	// The 16-bit KCP cmd/frg field (cmd 81-84, frg 0-255) cannot match the FEC
	// type markers 0x00f1-0x00f3.
	fecFlag := binary.LittleEndian.Uint16(data[4:])

	switch fecFlag {
	case typeData, typeParity: // packet with FEC
		if len(data) < fecHeaderSizePlus2 {
			DefaultSnmp.increaseInErrs()
			return
		}

		var kcpInErrors uint64
		f := fecPacket(data)

		// lock
		s.mu.Lock()
		defer s.mu.Unlock()

		// if fecDecoder is not initialized, create one with default parameter
		// lazy initialization
		if s.fecDecoder == nil {
			s.fecDecoder = newFECDecoder(1, 1)
		}

		// KCP input for data packets
		// only data packets are fed into kcp directly
		// parity packets are only used for recovery
		if f.flag() == typeData {
			if ret := s.kcp.Input(data[fecHeaderSizePlus2:], IKCP_PACKET_REGULAR, s.ackNoDelay); ret != 0 {
				kcpInErrors++
			}
		}

		// FEC decoding
		// If any packets were recovered by FEC, feed them into KCP.
		recovers := s.fecDecoder.decode(f)
		for _, r := range recovers {
			if len(r) >= 2 { // Must be at least 2 bytes.
				sz := binary.LittleEndian.Uint16(r)
				if int(sz) <= len(r) && sz >= 2 {
					if ret := s.kcp.Input(r[2:sz], IKCP_PACKET_FEC, s.ackNoDelay); ret != 0 {
						kcpInErrors++
					}
				}
			}
			// recycle the buffer
			defaultBufferPool.Put(r)
		}

		// to notify the readers to receive the data if there's any
		if n := s.kcp.PeekSize(); n > 0 {
			s.notifyReadEvent()
		}

		// to notify the writers if the window size allows to send more packets
		// and the remote window size is not full.
		waitsnd := s.kcp.WaitSnd()
		if waitsnd < int(s.kcp.snd_wnd) {
			s.notifyWriteEvent()
		}

		if kcpInErrors > 0 {
			DefaultSnmp.addKCPInErrors(kcpInErrors)
		}
	case typeOOB:
		// Count received OOB packet
		DefaultSnmp.increaseOOBPackets()
		// If an OOB callback is registered, invoke it synchronously.
		// The callback is responsible for ensuring non-blocking behavior.
		if callback := s.callbackForOOB.Load(); callback != nil {
			// Data layout: | FEC header (fecHeaderSizePlus2) | conv (4B) | OOB payload |
			callback.(OOBCallBackType)(data[fecHeaderSizePlus2+convSize:])
		}
	default: // packet without FEC
		s.mu.Lock()
		defer s.mu.Unlock()

		if ret := s.kcp.Input(data, IKCP_PACKET_REGULAR, s.ackNoDelay); ret != 0 {
			DefaultSnmp.addKCPInErrors(1)
		}

		if n := s.kcp.PeekSize(); n > 0 {
			s.notifyReadEvent()
		}

		waitsnd := s.kcp.WaitSnd()
		if waitsnd < int(s.kcp.snd_wnd) {
			s.notifyWriteEvent()
		}
		return
	}
}

// ResetRTO resets the retransmission timers for all pending segments.
//
// This method does not trigger a synchronous flush. Retransmission will
// naturally occur at the next background update tick, governed by the
// `interval` configured via SetNoDelay (default 100ms).
//
// It is intended for cases where the application knows connectivity
// has been restored and wants to resume data delivery without waiting
// for existing retransmission timers to expire.
func (s *UDPSession) ResetRTO() {
	s.mu.Lock()
	s.kcp.ResetRTO()
	s.mu.Unlock()
}

// -----------------------------------------------------------------------
// Listener: server-side session multiplexer
// -----------------------------------------------------------------------

type (
	// Listener accepts incoming connections.
	Listener struct {
		block        BlockCrypt     // block encryption
		dataShards   int            // number of FEC data shards
		parityShards int            // number of FEC parity shards
		conn         net.PacketConn // the underlying packet connection
		ownConn      bool           // true if we created conn internally, false if provided by caller

		sessions    map[string]*UDPSession // all sessions accepted by this Listener
		sessionLock sync.RWMutex
		chAccepts   chan *UDPSession // Listen() backlog

		die     chan struct{} // notify the listener has closed
		dieOnce sync.Once

		// socket error handling
		socketReadError     atomic.Value
		chSocketReadError   chan struct{}
		socketReadErrorOnce sync.Once

		acceptDeadline *deadline.Deadline
	}
)

// packetInput is the Listener's packet input handler.
// It decrypts the packet, demultiplexes by remote address,
// and dispatches to existing sessions or creates new ones.
func (l *Listener) packetInput(data []byte, addr net.Addr) {
	switch block := l.block.(type) {
	case nil:
	case *aeadCrypt:
		nonceSize := block.NonceSize()
		if len(data) < nonceSize+block.Overhead() {
			return
		}

		nonce := data[:nonceSize]
		ciphertext := data[nonceSize:]

		plaintext, err := block.Open(ciphertext[:0], nonce, ciphertext, nil)
		if err != nil {
			DefaultSnmp.increaseInCsumErrors()
			return
		}

		data = plaintext
	default:
		// decryption and crc32 check
		if len(data) < cryptHeaderSize {
			return
		}

		block.Decrypt(data, data)
		data = data[nonceSize:]

		checksum := crc32.ChecksumIEEE(data[crcSize:])
		if checksum != binary.LittleEndian.Uint32(data) {
			DefaultSnmp.increaseInCsumErrors()
			return
		}

		data = data[crcSize:]
	}

	// basic check for minimum packet size
	// NOTE: OOB allows sending small packets and even empty packets.
	if len(data) < min(IKCP_OVERHEAD, fecHeaderSizePlus2+convSize) {
		return
	}

	// look for existing session
	l.sessionLock.RLock()
	s, exist := l.sessions[addr.String()]
	l.sessionLock.RUnlock()

	var conv, sn uint32
	hasConv := false

	// try to get conversation id from the packet
	// The 16-bit KCP cmd/frg field (cmd 81-84, frg 0-255) cannot match the FEC
	// type markers 0x00f1-0x00f3.
	fecFlag := binary.LittleEndian.Uint16(data[4:])

	switch fecFlag {
	case typeData:
		// data packet of FEC, conversation id inside
		if len(data) < fecHeaderSizePlus2+IKCP_OVERHEAD {
			break
		}

		hasConv = true
		conv = binary.LittleEndian.Uint32(data[fecHeaderSizePlus2:])
		sn = binary.LittleEndian.Uint32(data[fecHeaderSizePlus2+IKCP_SN_OFFSET:])
	case typeParity:
		// parity packet of FEC, conversation id inside
	case typeOOB:
		// OOB packets always carry the conversation ID immediately after the FEC header.
		hasConv = true
		// Data layout: | FEC header (fecHeaderSizePlus2) | conv (4B) | OOB payload |
		conv = binary.LittleEndian.Uint32(data[fecHeaderSizePlus2:])
	default:
		// packet without FEC
		if len(data) < IKCP_OVERHEAD { // basic check for minimum kcp packet size
			return
		}
		hasConv = true
		conv = binary.LittleEndian.Uint32(data)
		sn = binary.LittleEndian.Uint32(data[IKCP_SN_OFFSET:])
	}

	// on an existing connection
	if exist {
		// If we have a valid conversation id or we cannot get conversation id from the packet,
		// just feed the data into the existing session.
		if !hasConv || conv == s.kcp.conv {
			s.kcpInput(data)
			return
		}
		// conversation id mismatched, only accept reset packet with sn == 0
		if sn != 0 {
			return
		}
		// Close will remove the session from listener's session map,
		// So we can create a new session with the same addr below.
		s.Close()
	}

	// The connection does not exist, try to create a new one.
	// But if we don't have a valid conversation id, nothing we can do here except dropping the packet.
	if !hasConv {
		return
	}

	// Now we have a valid conversation id here without a session object, create a new session.
	// do not let the new sessions overwhelm accept queue
	if len(l.chAccepts) >= cap(l.chAccepts) {
		return
	}

	// new session
	s = newUDPSession(conv, l.dataShards, l.parityShards, l, l.conn, false, addr, l.block)
	s.kcpInput(data)
	l.sessionLock.Lock()
	select {
	case <-l.die:
		l.sessionLock.Unlock()
		s.Close()
		return
	default:
	}
	l.sessions[addr.String()] = s
	l.sessionLock.Unlock()
	l.chAccepts <- s
}

func (l *Listener) notifyReadError(err error) {
	l.socketReadErrorOnce.Do(func() {
		l.socketReadError.Store(err)
		close(l.chSocketReadError)

		// propagate read error to all sessions
		l.sessionLock.RLock()
		for _, s := range l.sessions {
			s.notifyReadError(err)
		}
		l.sessionLock.RUnlock()
	})
}

// SetReadBuffer sets the socket read buffer for the Listener
func (l *Listener) SetReadBuffer(bytes int) error {
	if conn, ok := l.conn.(setReadBuffer); ok {
		return conn.SetReadBuffer(bytes)
	}
	return errInvalidOperation
}

// SetWriteBuffer sets the socket write buffer for the Listener
func (l *Listener) SetWriteBuffer(bytes int) error {
	if conn, ok := l.conn.(setWriteBuffer); ok {
		return conn.SetWriteBuffer(bytes)
	}
	return errInvalidOperation
}

// SetDSCP sets the 6-bit DSCP field in the IPv4 header or the 6-bit Traffic
// Class field in the IPv6 header.
//
// If the underlying connection implements `SetDSCP(int) error`, SetDSCP invokes
// that method instead.
func (l *Listener) SetDSCP(dscp int) error {
	// interface enabled
	if conn, ok := l.conn.(setDSCP); ok {
		return conn.SetDSCP(dscp)
	}

	conn, ok := l.conn.(net.Conn)
	if !ok {
		return errInvalidOperation
	}

	val := dscp << 2
	var succeed bool
	if err := ipv4.NewConn(conn).SetTOS(val); err == nil {
		succeed = true
	}

	if err := ipv6.NewConn(conn).SetTrafficClass(val); err == nil {
		succeed = true
	}

	if succeed {
		return nil
	}

	return errInvalidOperation
}

// Accept waits for and returns the next connection as a net.Conn.
func (l *Listener) Accept() (net.Conn, error) {
	return l.AcceptKCP()
}

// AcceptKCP accepts a KCP connection
func (l *Listener) AcceptKCP() (*UDPSession, error) {
	select {
	case <-l.acceptDeadline.Done():
		return nil, errors.WithStack(errTimeout)
	case c := <-l.chAccepts:
		return c, nil
	case <-l.chSocketReadError:
		return nil, l.socketReadError.Load().(error)
	case <-l.die:
		return nil, errors.WithStack(io.ErrClosedPipe)
	}
}

// SetDeadline sets the deadline associated with the listener. A zero time value disables the deadline.
func (l *Listener) SetDeadline(t time.Time) error {
	return l.SetReadDeadline(t)
}

// SetReadDeadline implements the Conn SetReadDeadline method.
func (l *Listener) SetReadDeadline(t time.Time) error {
	l.acceptDeadline.Set(t)
	return nil
}

// SetWriteDeadline implements the Conn SetWriteDeadline method.
func (l *Listener) SetWriteDeadline(t time.Time) error {
	return errInvalidOperation
}

// Close stops listening on the UDP address and closes the socket if the
// Listener owns it.
func (l *Listener) Close() error {
	var once bool
	l.sessionLock.Lock()
	l.dieOnce.Do(func() {
		close(l.die)
		once = true
	})

	if !once {
		l.sessionLock.Unlock()
		return errors.WithStack(io.ErrClosedPipe)
	}

	pp := l.releasePusherLocked()
	l.sessionLock.Unlock()
	if pp != nil {
		pp.SetPacketReceiver(nil)
	}

	if l.ownConn {
		return l.conn.Close()
	}

	return nil
}

// Control applies a function to the underlying packet connection.
// CAUTION: BE VERY CAREFUL TO USE THIS FUNCTION, YOU MAY BREAK THE PROTOCOL.
func (l *Listener) Control(f func(conn net.PacketConn) error) error {
	l.sessionLock.Lock()
	defer l.sessionLock.Unlock()

	return f(l.conn)
}

// closeSession notifies the listener that a session has closed.
func (l *Listener) closeSession(remote net.Addr) (ret bool) {
	l.sessionLock.Lock()
	if _, ok := l.sessions[remote.String()]; ok {
		delete(l.sessions, remote.String())
		pp := l.releasePusherLocked()
		l.sessionLock.Unlock()
		if pp != nil {
			pp.SetPacketReceiver(nil)
		}
		return true
	}
	l.sessionLock.Unlock()
	return false
}

// releasePusherLocked removes the PacketPusher receiver once the Listener is
// closed and its last session has gone. sessionLock must be held. The caller
// must unregister the returned pusher after releasing sessionLock.
func (l *Listener) releasePusherLocked() PacketPusher {
	select {
	case <-l.die:
	default:
		return nil
	}
	if len(l.sessions) > 0 {
		return nil
	}
	if pp, ok := l.conn.(PacketPusher); ok {
		return pp
	}
	return nil
}

// Addr returns the listener's network address. The returned address is shared
// by all invocations of Addr, so do not modify it.
func (l *Listener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

// -----------------------------------------------------------------------
// Public API: Dial, Listen, and connection factory functions
// -----------------------------------------------------------------------

// Listen listens for incoming KCP packets addressed to the local address laddr on the network "udp",
func Listen(laddr string) (net.Listener, error) {
	return ListenWithOptions(laddr, nil, 0, 0)
}

// ListenWithOptions listens for incoming KCP packets addressed to laddr on
// the "udp" network, with optional packet encryption and FEC.
//
// 'block' is the block encryption algorithm to encrypt packets.
//
// dataShards and parityShards specify the numbers of data and parity shards,
// respectively.
//
// See https://github.com/klauspost/reedsolomon for details.
func ListenWithOptions(laddr string, block BlockCrypt, dataShards, parityShards int) (*Listener, error) {
	udpaddr, err := net.ResolveUDPAddr("udp", laddr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	conn, err := net.ListenUDP("udp", udpaddr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return serveConn(block, dataShards, parityShards, conn, true)
}

// ServeConn serves the KCP protocol over a single packet connection.
func ServeConn(block BlockCrypt, dataShards, parityShards int, conn net.PacketConn) (*Listener, error) {
	return serveConn(block, dataShards, parityShards, conn, false)
}

func serveConn(block BlockCrypt, dataShards, parityShards int, conn net.PacketConn, ownConn bool) (*Listener, error) {
	l := new(Listener)
	l.conn = conn
	l.ownConn = ownConn
	l.sessions = make(map[string]*UDPSession)
	l.chAccepts = make(chan *UDPSession, acceptBacklog)
	l.die = make(chan struct{})
	l.dataShards = dataShards
	l.parityShards = parityShards
	l.block = block
	l.chSocketReadError = make(chan struct{})

	l.acceptDeadline = deadline.New()

	// Stop the Accept deadline timer when the listener is closed.
	runtime.AddCleanup(l, freeDeadline, l.acceptDeadline)

	if pp, ok := conn.(PacketPusher); ok {
		pp.SetPacketReceiver(l.pushedPacket)
	} else {
		go l.monitor()
	}
	return l, nil
}

// Dial connects to the remote address "raddr" on the network "udp" without encryption and FEC
func Dial(raddr string) (net.Conn, error) {
	return DialWithOptions(raddr, nil, 0, 0)
}

// DialWithOptions connects to raddr on the "udp" network, with optional packet
// encryption and FEC.
//
// 'block' is the block encryption algorithm to encrypt packets.
//
// dataShards and parityShards specify the numbers of data and parity shards,
// respectively.
//
// See https://github.com/klauspost/reedsolomon for details.
func DialWithOptions(raddr string, block BlockCrypt, dataShards, parityShards int) (*UDPSession, error) {
	// network type detection
	udpaddr, err := net.ResolveUDPAddr("udp", raddr)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	network := "udp4"
	if udpaddr.IP.To4() == nil {
		network = "udp"
	}

	conn, err := net.ListenUDP(network, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var convid uint32
	binary.Read(rand.Reader, binary.LittleEndian, &convid)
	return newUDPSession(convid, dataShards, parityShards, nil, conn, true, udpaddr, block), nil
}

// NewConn4 establishes a session that speaks KCP over a packet connection.
func NewConn4(convid uint32, raddr net.Addr, block BlockCrypt, dataShards, parityShards int, ownConn bool, conn net.PacketConn) (*UDPSession, error) {
	return newUDPSession(convid, dataShards, parityShards, nil, conn, ownConn, raddr, block), nil
}

// NewConn3 establishes a session that speaks KCP over a packet connection.
func NewConn3(convid uint32, raddr net.Addr, block BlockCrypt, dataShards, parityShards int, conn net.PacketConn) (*UDPSession, error) {
	return newUDPSession(convid, dataShards, parityShards, nil, conn, false, raddr, block), nil
}

// NewConn2 establishes a session that speaks KCP over a packet connection.
func NewConn2(raddr net.Addr, block BlockCrypt, dataShards, parityShards int, conn net.PacketConn) (*UDPSession, error) {
	var convid uint32
	binary.Read(rand.Reader, binary.LittleEndian, &convid)
	return NewConn3(convid, raddr, block, dataShards, parityShards, conn)
}

// NewConn establishes a session that speaks KCP over a packet connection.
func NewConn(raddr string, block BlockCrypt, dataShards, parityShards int, conn net.PacketConn) (*UDPSession, error) {
	udpaddr, err := net.ResolveUDPAddr("udp", raddr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return NewConn2(udpaddr, block, dataShards, parityShards, conn)
}

// buffersEmpty reports whether v holds no bytes.
func buffersEmpty(v [][]byte) bool {
	for _, b := range v {
		if len(b) > 0 {
			return false
		}
	}
	return true
}

func (s *UDPSession) notifyReadIfAvailableLocked() {
	if s.input.Len() > 0 || s.kcp.PeekSize() > 0 {
		s.notifyReadEvent()
	}
}

// TryRead is Read without waiting. It returns 0 and no error when no data is
// ready, and the session's error once it is closed or its socket failed and
// no data is left.
func (s *UDPSession) TryRead(b []byte) (int, error) {
	n, ok := s.read(b)
	if ok {
		return n, nil
	}

	select {
	case <-s.chSocketReadError:
		return 0, s.socketReadError.Load().(error)
	case <-s.die:
		return 0, errors.WithStack(io.ErrClosedPipe)
	default:
		return 0, nil
	}
}

// SetReadNotify sets fn to be called whenever data may have become ready for
// TryRead, and when the session closes or its socket fails. fn runs with the
// session locked, so it must not block or call into the session.
func (s *UDPSession) SetReadNotify(fn func()) {
	s.readNotify.Store(&fn)
}

func (s *UDPSession) callReadNotify() {
	if fn := s.readNotify.Load(); fn != nil {
		(*fn)()
	}
}
