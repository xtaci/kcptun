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

const maxAutoTuneSamples = 258

// pulse represents a 0/1 signal with time sequence
type pulse struct {
	bit bool   // 0 or 1
	seq uint32 // sequence of the signal
}

// autoTune object to detect pulses in a signal
type autoTune struct {
	pulses [maxAutoTuneSamples]pulse
}

// Sample adds a signal sample to the pulse buffer
func (tune *autoTune) Sample(bit bool, seq uint32) {
	// ensure seq is in range [pulses[0].seq, pulses[0].seq + maxAutoTuneSamples]
	if seq >= tune.pulses[0].seq && seq <= tune.pulses[0].seq+maxAutoTuneSamples {
		tune.pulses[seq%maxAutoTuneSamples] = pulse{bit, seq}
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
	// last pulse and initial index setup
	lastPulse := tune.pulses[0]
	idx := 1

	// left edge
	var leftEdge int
	for ; idx < len(tune.pulses); idx++ {
		if lastPulse.bit != bit && tune.pulses[idx].bit == bit { // edge found
			if lastPulse.seq+1 == tune.pulses[idx].seq { // ensure edge continuity
				leftEdge = idx
				break
			}
		}
		lastPulse = tune.pulses[idx]
	}

	// right edge
	var rightEdge int
	lastPulse = tune.pulses[leftEdge]
	idx = leftEdge + 1

	for ; idx < len(tune.pulses); idx++ {
		if lastPulse.seq+1 == tune.pulses[idx].seq { // ensure pulses in this level monotonic
			if lastPulse.bit == bit && tune.pulses[idx].bit != bit { // edge found
				rightEdge = idx
				break
			}
		} else {
			return -1
		}
		lastPulse = tune.pulses[idx]
	}

	return rightEdge - leftEdge
}
