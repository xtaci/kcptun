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

import "sync"

// sendQueueKeep is the maximum slice capacity retained after the queue drains.
const sendQueueKeep = 64

// sendQueue is a bounded FIFO of packets awaiting post-processing. It grows
// its backing slice on demand and drops oversized storage after draining.
type sendQueue struct {
	mu   sync.Mutex
	q    []sendRequest
	head int
}

// push appends r unless devBacklog requests are already queued.
func (sq *sendQueue) push(r sendRequest) bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	if len(sq.q)-sq.head >= devBacklog {
		return false
	}
	if sq.head > 0 && len(sq.q) == cap(sq.q) {
		n := copy(sq.q, sq.q[sq.head:])
		clear(sq.q[n:])
		sq.q, sq.head = sq.q[:n], 0
	}
	sq.q = append(sq.q, r)
	return true
}

// pop removes the oldest packet and releases the backing slice after draining
// if its capacity exceeds sendQueueKeep.
func (sq *sendQueue) pop() (sendRequest, bool) {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	if sq.head == len(sq.q) {
		return sendRequest{}, false
	}
	r := sq.q[sq.head]
	sq.q[sq.head] = sendRequest{}
	sq.head++
	if sq.head == len(sq.q) {
		sq.q, sq.head = sq.q[:0], 0
		if cap(sq.q) > sendQueueKeep {
			sq.q = nil
		}
	}
	return r, true
}

// len returns the number of queued packets.
func (sq *sendQueue) len() int {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return len(sq.q) - sq.head
}
