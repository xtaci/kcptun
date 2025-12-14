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
	"container/heap"
	"sort"
)

const maxAutoTuneSamples = 258 // 256 + 2 extra for edge detection

// pulse represents a 0/1 signal with time sequence
type pulse struct {
	bit bool   // 0 or 1
	seq uint32 // sequence of the signal
}

// pulseHeap is a min-heap structure, ordered by the sequence number (seq).
type pulseHeap []pulse

func (h pulseHeap) Len() int { return len(h) }
func (h pulseHeap) Less(i, j int) bool { // Min-heap: smaller seq goes to the top
	return _itimediff(h[i].seq, h[j].seq) < 0
}

func (h pulseHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *pulseHeap) Push(x interface{}) {
	*h = append(*h, x.(pulse))
}

func (h *pulseHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// autoTune object to detect pulses in a signal
type autoTune struct {
	pulses pulseHeap
}

// Sample adds a signal sample to the pulse buffer
func (tune *autoTune) Sample(bit bool, seq uint32) {
	// 1. Push the new sample onto the heap
	heap.Push(&tune.pulses, pulse{
		bit: bit,
		seq: seq,
	})

	// 2. Maintain the maximum capacity
	// If the capacity is exceeded, pop the heap's root element (the packet with the smallest/oldest seq).
	// This ensures the heap always contains the latest 258 packets in terms of sequence number.
	if tune.pulses.Len() > maxAutoTuneSamples {
		heap.Pop(&tune.pulses)
	}
}

// Find a period for a given signal
// returns -1 if not found
//
//
//   Signal Level
//       |
// 1.0   |                 _____           _____
//       |                |     |         |     |
// 0.5   |      _____     |     |   _____ |     |   _____
//       |     |     |    |     |  |     ||     |  |     |
// 0.0 __|_____|     |____|     |__|     ||     |__|     |_____
//       |
//       |-----------------------------------------------------> Time
//            A     B    C     D  E     F     G  H     I

func (tune *autoTune) FindPeriod(bit bool) int {
	// Need at least 3 samples to detect a period (rising and falling edges)
	if tune.pulses.Len() < 3 {
		return -1
	}

	// Copy the underlying array for sorting and analysis.
	// This avoids modifying the heap structure.
	sorted := make([]pulse, len(tune.pulses))
	copy(sorted, tune.pulses)

	// Sort the copied data by sequence number (seq) to ensure linear order for period calculation.
	sort.Slice(sorted, func(i, j int) bool {
		return _itimediff(sorted[i].seq, sorted[j].seq) < 0
	})

	// left edge
	leftEdge := -1
	lastPulse := sorted[0]
	idx := 1

	for ; idx < len(sorted); idx++ {
		if lastPulse.seq+1 == sorted[idx].seq { // continuous sequence
			if lastPulse.bit != bit && sorted[idx].bit == bit { // edge found
				leftEdge = idx // mark left edge(the changed bit position)
				break
			}
		} else {
			return -1
		}
		lastPulse = sorted[idx]
	}

	// no left edge found
	if leftEdge == -1 {
		return -1
	}

	// right edge
	rightEdge := -1
	lastPulse = sorted[leftEdge]
	idx = leftEdge + 1

	for ; idx < len(sorted); idx++ {
		if lastPulse.seq+1 == sorted[idx].seq {
			if lastPulse.bit == bit && sorted[idx].bit != bit {
				rightEdge = idx
				break
			}
		} else {
			return -1
		}
		lastPulse = sorted[idx]
	}

	// no right edge found
	if rightEdge == -1 {
		return -1
	}

	return rightEdge - leftEdge
}
