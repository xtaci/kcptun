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
	"sync"
	"time"
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

func (h shaperHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *shaperHeap) Push(x interface{}) { *h = append(*h, x.(writeRequest)) }

func (h *shaperHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

const (
	streamExpireDuration = 1 * time.Minute
)

type shaperQueue struct {
	streams    map[uint32]*shaperHeap
	lastVisits map[uint32]time.Time
	allSids    []uint32
	nextIdx    uint32
	count      uint32
	mu         sync.Mutex
}

func NewShaperQueue() *shaperQueue {
	return &shaperQueue{
		streams:    make(map[uint32]*shaperHeap),
		lastVisits: make(map[uint32]time.Time),
	}
}

func (sq *shaperQueue) Push(req writeRequest) {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	sid := req.frame.sid
	if _, ok := sq.streams[sid]; !ok {
		sq.streams[sid] = new(shaperHeap)
		sq.allSids = append(sq.allSids, sid)
	}
	h := sq.streams[sid]
	heap.Push(h, req)
	sq.lastVisits[sid] = time.Now()
	sq.count++
}

// Pop uses Round Robin to pop writeRequests from the shaperQueue.
func (sq *shaperQueue) Pop() (req writeRequest, ok bool) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.allSids) == 0 {
		return writeRequest{}, false
	}

	start := sq.nextIdx % uint32(len(sq.allSids))

	// loop through all streams in a round-robin manner
	for i := 0; i < len(sq.allSids); i++ {
		idx := (int(start) + i) % len(sq.allSids)
		sid := sq.allSids[idx]
		h := sq.streams[sid]
		if h == nil || h.Len() == 0 {
			continue
		}

		// pop from the heap
		req := heap.Pop(h).(writeRequest)
		sq.count--

		// If the heap is empty after popping, remove it from the map
		if h.Len() == 0 && sq.lastVisits[sid].Add(streamExpireDuration).Before(time.Now()) {
			delete(sq.streams, sid)
			delete(sq.lastVisits, sid)
			// copy the rest of allSids to overwrite the removed sid
			sq.allSids = append(sq.allSids[:idx], sq.allSids[idx+1:]...)
		}

		// update nextSid for round-robin
		if len(sq.allSids) == 0 {
			sq.nextIdx = 0
		} else {
			sq.nextIdx = uint32((idx + 1) % len(sq.allSids))
		}
		return req, true
	}

	return writeRequest{}, false
}

func (sq *shaperQueue) IsEmpty() bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return sq.count == 0
}

func (sq *shaperQueue) Len() int {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return int(sq.count)
}
