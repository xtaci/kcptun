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
	"container/heap"
	"container/list"
	"sync"
	"sync/atomic"
)

// _itimediff returns the time difference between two uint32 values.
// The result is a signed 32-bit integer representing the difference between 'later' and 'earlier'.
func _itimediff(later, earlier uint32) int32 {
	return (int32)(later - earlier)
}

// shaperHeap is a min-heap of writeRequest.
// It orders writeRequests by class first, then by sequence number within the same class.
type shaperHeap []writeRequest

func (h shaperHeap) Len() int { return len(h) }

// Less determines the ordering of elements in the heap.
// Requests are ordered by their class first. If two requests have the same class,
// they are ordered by their sequence numbers.
func (h shaperHeap) Less(i, j int) bool {
	if h[i].class != h[j].class {
		return h[i].class < h[j].class
	}
	return _itimediff(h[j].seq, h[i].seq) > 0
}

func (h shaperHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *shaperHeap) Push(x any)   { *h = append(*h, x.(writeRequest)) }

func (h *shaperHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	old[n-1] = writeRequest{} // avoid memory leak
	*h = old[0 : n-1]
	return x
}

// shaperQueue manages multiple streams of writeRequests using a round-robin scheduling algorithm.
type shaperQueue struct {
	count   int64 // atomic counter for fast Len() and IsEmpty()
	streams map[uint32]*shaperHeap
	rrList  *list.List    // list of sid (RR queue)
	next    *list.Element // next node to pop
	mu      sync.Mutex
}

// shaperHeapPool reduces allocation of shaperHeap objects
var shaperHeapPool = sync.Pool{
	New: func() any {
		h := make(shaperHeap, 0, 16) // pre-allocate capacity
		return &h
	},
}

func NewShaperQueue() *shaperQueue {
	return &shaperQueue{
		streams: make(map[uint32]*shaperHeap),
		rrList:  list.New(),
	}
}

// Push adds a writeRequest to the shaperQueue.
func (sq *shaperQueue) Push(req writeRequest) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	// create heap for the stream if not exists.
	sid := req.frame.sid
	if _, ok := sq.streams[sid]; !ok {
		// get heap from pool
		h := shaperHeapPool.Get().(*shaperHeap)
		*h = (*h)[:0] // reset while keeping capacity
		sq.streams[sid] = h
		elem := sq.rrList.PushBack(sid)
		if sq.next == nil {
			sq.next = elem
		}
	}

	// push the request into the corresponding stream heap.
	h := sq.streams[sid]
	heap.Push(h, req)
	atomic.AddInt64(&sq.count, 1)
}

// Pop uses Round Robin to pop writeRequests from the shaperQueue.
func (sq *shaperQueue) Pop() (req writeRequest, ok bool) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	// if there are no streams, return false
	if sq.next == nil || atomic.LoadInt64(&sq.count) == 0 {
		return writeRequest{}, false
	}

	// get the starting index for round-robin.
	start := sq.next
	current := start

	// loop through all streams in a round-robin manner
	for {
		sid := current.Value.(uint32)
		h := sq.streams[sid]

		if h.Len() > 0 {
			// pop the top request from the heap
			req := heap.Pop(h).(writeRequest)
			atomic.AddInt64(&sq.count, -1)

			// update next pointer for round-robin
			next := current.Next()
			if next == nil {
				next = sq.rrList.Front()
			}
			sq.next = next

			// If the heap is empty after popping, delete it.
			if h.Len() == 0 {
				delete(sq.streams, sid)
				sq.rrList.Remove(current)
				// return heap to pool
				shaperHeapPool.Put(h)
				// if a list has only one element, then current->next will point to itself,
				// so after removing current, we need to set next to nil.
				if sq.rrList.Len() == 0 {
					sq.next = nil
				}
			}
			return req, true
		}

		// move to next
		current = current.Next()
		if current == nil {
			current = sq.rrList.Front()
		}
		if current == start { // full loop: no packets
			break
		}
	}

	// no requests found in any stream
	return writeRequest{}, false
}

// IsEmpty checks if the shaperQueue is empty.
func (sq *shaperQueue) IsEmpty() bool {
	return atomic.LoadInt64(&sq.count) == 0
}

// Len returns the total number of writeRequests in the shaperQueue.
func (sq *shaperQueue) Len() int {
	return int(atomic.LoadInt64(&sq.count))
}
