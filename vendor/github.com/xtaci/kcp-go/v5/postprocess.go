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

package kcp

import (
	"runtime"
	"sync"

	"golang.org/x/net/ipv4"
)

// ppBudget is how many packets a worker takes from one session before it
// moves on, so a busy session cannot starve the others.
const ppBudget = 4 * maxBatchSize

// ppQueue holds the sessions with packets waiting for postProcess. A fixed
// set of workers serves every session, so an idle session holds no goroutine.
// ppRunning ensures that a session appears at most once in this queue or is
// being handled by a worker. head avoids shifting entries after each dequeue.
var ppQueue struct {
	once sync.Once
	mu   sync.Mutex
	cond *sync.Cond
	q    []*UDPSession
	head int
}

// ppWorker owns a batch while a worker transforms queued packets into UDP
// datagrams. Its slices are reused between sessions to reduce allocations.
type ppWorker struct {
	txqueue []ipv4.Message
	bufs    [][]byte
}

// oneBuf appends b to the worker-owned slice and returns a one-element view
// suitable for ipv4.Message.Buffers. recycle clears that view before reuse.
func (w *ppWorker) oneBuf(b []byte) [][]byte {
	w.bufs = append(w.bufs, b)
	return w.bufs[len(w.bufs)-1 : len(w.bufs) : len(w.bufs)]
}

// recycle returns every datagram buffer to the shared pool and clears
// references held by the worker's message and slice arrays.
func (w *ppWorker) recycle() {
	for k := range w.txqueue {
		defaultBufferPool.Put(w.txqueue[k].Buffers[0])
		w.txqueue[k].Buffers = nil
	}
	w.txqueue = w.txqueue[:0]
	clear(w.bufs)
	w.bufs = w.bufs[:0]
}

// ppWorkers reuses batch storage across post-processing workers.
var ppWorkers = sync.Pool{New: func() any {
	return &ppWorker{
		txqueue: make([]ipv4.Message, 0, maxBatchSize),
		bufs:    make([][]byte, 0, maxBatchSize),
	}
}}

// flushTx sends a completed batch and reports whether the worker must discard
// it because the session is closed.
func (s *UDPSession) flushTx(w *ppWorker, bytesToSend int) bool {
	if s.isClosed() {
		w.recycle()
		ppWorkers.Put(w)
		return true
	}
	s.sendTx(w, bytesToSend)
	return false
}

// sendTx performs the platform-specific send for a completed batch, emits a
// trace record when enabled, and recycles the batch buffers. ppSendMu keeps
// Close from returning while a send is in progress and rejects sends after
// Close has shut the gate.
func (s *UDPSession) sendTx(w *ppWorker, bytesToSend int) bool {
	s.ppSendMu.Lock()
	defer s.ppSendMu.Unlock()
	if s.ppClosing {
		w.recycle()
		return false
	}
	s.tx(w.txqueue)
	if kcpTrace {
		s.mu.Lock()
		s.kcp.debugLog(IKCP_LOG_OUTPUT, "conv", s.kcp.conv, "datalen", bytesToSend)
		s.mu.Unlock()
	}
	w.recycle()
	return true
}

// kickPostProcess schedules s after its caller enqueues a packet. The atomic
// claim prevents concurrent producers from queueing the same session twice.
func (s *UDPSession) kickPostProcess() {
	if s.claimPostProcess() {
		ppEnqueue(s)
	}
}

// claimPostProcess changes the session from idle to queued-or-running.
func (s *UDPSession) claimPostProcess() bool {
	return s.ppRunning.CompareAndSwap(false, true)
}

// finishPostProcess relinquishes the current worker claim. If the session is
// still open and packets arrived during processing, it claims the session
// again so the caller can enqueue another worker pass. Closed sessions instead
// release queued packet buffers.
func (s *UDPSession) finishPostProcess() bool {
	s.ppRunning.Store(false)
	if s.isClosed() {
		s.discardPostProcess()
		return false
	}
	return s.postQueue.len() > 0 && s.claimPostProcess()
}

// discardPostProcess releases queued packet buffers after Close.
// Close serializes its final KCP flush and closes die while holding s.mu, so
// the output callback cannot enqueue more packets after this drain completes.
func (s *UDPSession) discardPostProcess() {
	for {
		req, ok := s.postQueue.pop()
		if !ok {
			return
		}
		defaultBufferPool.Put(req.buffer)
	}
}

// ppEnqueue appends a claimed session to the shared worker queue and wakes one
// worker. Queue compaction drops consumed references without shifting on every
// dequeue.
func ppEnqueue(s *UDPSession) {
	ppQueue.once.Do(startPostProcessWorkers)
	ppQueue.mu.Lock()
	if ppQueue.head > len(ppQueue.q)/2 {
		n := copy(ppQueue.q, ppQueue.q[ppQueue.head:])
		clear(ppQueue.q[n:])
		ppQueue.q, ppQueue.head = ppQueue.q[:n], 0
	}
	ppQueue.q = append(ppQueue.q, s)
	ppQueue.mu.Unlock()
	ppQueue.cond.Signal()
}

// startPostProcessWorkers initializes the queue condition variable and starts
// a bounded process-wide pool. It is called exactly once by ppQueue.once.
func startPostProcessWorkers() {
	ppQueue.cond = sync.NewCond(&ppQueue.mu)
	n := min(max(runtime.NumCPU(), 2), 8)
	for i := 0; i < n; i++ {
		go postProcessWorker()
	}
}

// postProcessWorker repeatedly takes one claimed session from ppQueue,
// processes packets within the per-session budget, and requeues the session
// when more packets remain.
func postProcessWorker() {
	w := ppWorkers.Get().(*ppWorker)
	for {
		ppQueue.mu.Lock()
		for ppQueue.head == len(ppQueue.q) {
			ppQueue.cond.Wait()
		}
		s := ppQueue.q[ppQueue.head]
		ppQueue.q[ppQueue.head] = nil
		ppQueue.head++
		if ppQueue.head == len(ppQueue.q) {
			ppQueue.q, ppQueue.head = ppQueue.q[:0], 0
		}
		ppQueue.mu.Unlock()

		if s.isClosed() {
			// Close may leave a queue entry behind after draining its buffers.
			// finishPostProcess clears the claim and performs an idempotent drain.
			s.finishPostProcess()
			continue
		}

		more := s.postProcess(w)
		if more {
			ppEnqueue(s)
			continue
		}
		// Hand back to kickPostProcess, unless a packet arrived meanwhile.
		if s.finishPostProcess() {
			ppEnqueue(s)
		}
	}
}
