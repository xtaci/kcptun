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
	"sort"
)

const maxAutoTuneSamples = 258 // 256 + 2 extra samples for edge detection

// pulse represents a single sample in the signal stream: a boolean bit
// (data=true, parity=false) tagged with its FEC sequence number.
type pulse struct {
	bit bool   // true = data shard, false = parity shard
	seq uint32 // FEC sequence number of this packet
}

// autoTune detects the repeating period of data and parity shards in the
// incoming FEC stream. This allows the decoder to auto-detect the peer's
// dataShards/parityShards configuration.
//
// Algorithm: collect recent samples into a ring buffer, sort by sequence,
// then scan for the first complete pulse (rising edge -> falling edge)
// of the target signal. The pulse width equals the shard count.
type autoTune struct {
	pulses    [maxAutoTuneSamples]pulse // fixed-size array to avoid heap allocations
	sortCache [maxAutoTuneSamples]pulse // reusable cache for sorting to avoid allocations
	head      int                       // oldest element index
	tail      int                       // next write position
	count     int                       // number of elements
}

// Sample adds a signal sample to the pulse buffer using a ring buffer
func (tune *autoTune) Sample(bit bool, seq uint32) {
	// Write to current tail position
	tune.pulses[tune.tail] = pulse{bit: bit, seq: seq}
	tune.tail = (tune.tail + 1) % maxAutoTuneSamples

	if tune.count < maxAutoTuneSamples {
		tune.count++
	} else {
		// Buffer is full, advance head (discard oldest)
		tune.head = (tune.head + 1) % maxAutoTuneSamples
	}
}

// FindPeriod detects the period (pulse width) of the given signal type
// in the collected samples. Returns -1 if no valid period is found.
//
// For bit=true (data shards), it finds how many consecutive data shards
// appear before a parity shard, giving us the dataShards count.
// For bit=false (parity shards), it gives the parityShards count.
//
// The detection works by finding a complete pulse in the sorted sequence:
//  1. Find the "left edge": where the signal transitions TO the target bit
//  2. Find the "right edge": where the signal transitions AWAY from the target bit
//  3. The period = rightEdge - leftEdge
//
// Example signal (bit=true looking for data period):
//
//	Signal Level
//	    |
//	1.0 |                 _____           _____
//	    |                |     |         |     |
//	0.5 |      _____     |     |   _____ |     |   _____
//	    |     |     |    |     |  |     ||     |  |     |
//	0.0 |_____|     |____|     |__|     ||     |__|     |_____
//	    |
//	    |-----------------------------------------------------> Time
//	         A     B    C     D  E     F     G  H     I
func (tune *autoTune) FindPeriod(bit bool) int {
	// Need at least 3 samples to detect a period (rising and falling edges)
	if tune.count < 3 {
		return -1
	}

	// Copy elements from ring buffer to sortCache for sorting and analysis.
	// Using fixed-size array to avoid heap allocation.
	for i := 0; i < tune.count; i++ {
		idx := (tune.head + i) % maxAutoTuneSamples
		tune.sortCache[i] = tune.pulses[idx]
	}

	// Create a slice view over the cache for sorting
	sorted := tune.sortCache[:tune.count]

	// Sort the copied data by sequence number (seq) to ensure linear order for period calculation.
	sort.Slice(sorted, func(i, j int) bool {
		return _itimediff(sorted[i].seq, sorted[j].seq) < 0
	})

	// Collapse duplicate sequence numbers. Conflicting samples for the same
	// sequence are unusable and are discarded.
	unique := 0
	for i := 0; i < len(sorted); {
		end := i + 1
		conflict := false
		for end < len(sorted) && sorted[end].seq == sorted[i].seq {
			conflict = conflict || sorted[end].bit != sorted[i].bit
			end++
		}
		if !conflict {
			sorted[unique] = sorted[i]
			unique++
		}
		i = end
	}
	sorted = sorted[:unique]

	// Find a complete pulse with consecutive samples from the entering edge
	// through the exiting edge. A gap can hide a signal transition, making
	// the pulse width ambiguous.
	for left := 1; left < len(sorted); left++ {
		if sorted[left-1].bit == bit || sorted[left].bit != bit ||
			sorted[left-1].seq+1 != sorted[left].seq {
			continue
		}

		for right := left + 1; right < len(sorted); right++ {
			if sorted[right-1].seq+1 != sorted[right].seq {
				break
			}
			if sorted[right].bit != bit {
				return int(_itimediff(sorted[right].seq, sorted[left].seq))
			}
		}
	}

	return -1
}
