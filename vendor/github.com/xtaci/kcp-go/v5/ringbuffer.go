// The MIT License (MIT)
//
// Copyright (c) 2025 xtaci
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

const (
	RINGBUFFER_MIN = 8    // minimum backing-slice length
	RINGBUFFER_EXP = 1024 // backing-slice growth threshold: double below; add 10% at or above
)

// RingBuffer is a generic ring (circular) buffer that supports dynamic resizing.
// It provides efficient FIFO queue behavior with amortized constant time operations.
type RingBuffer[T any] struct {
	head     int // Index of the next element to be popped
	tail     int // Index of the next empty slot to push into
	elements []T // Underlying slice storing elements in circular fashion
}

// NewRingBuffer creates a ring buffer with a backing slice of the specified
// size. One slot is reserved to distinguish full from empty, so MaxLen is
// size-1. Sizes of 8 or less use a backing slice of length 8.
func NewRingBuffer[T any](size int) *RingBuffer[T] {
	if size <= RINGBUFFER_MIN {
		size = RINGBUFFER_MIN // Ensure a minimum size
	}
	return &RingBuffer[T]{
		head:     0,
		tail:     0,
		elements: make([]T, size),
	}
}

// Len returns the number of elements currently in the ring.
func (r *RingBuffer[T]) Len() int {
	if r.head <= r.tail {
		return r.tail - r.head
	}
	// Wrapped case: elements from head to end + elements from start to tail
	return len(r.elements) - r.head + r.tail
}

// Push adds an element to the tail of the ring.
// If the ring is full, it will grow automatically.
func (r *RingBuffer[T]) Push(v T) {
	if r.IsFull() {
		r.grow()
	}
	r.elements[r.tail] = v
	r.tail = (r.tail + 1) % len(r.elements)
}

// Pop removes and returns the element from the head of the ring.
// It returns the zero value and false if the ring is empty.
func (r *RingBuffer[T]) Pop() (T, bool) {
	var zero T
	if r.Len() == 0 {
		return zero, false
	}
	value := r.elements[r.head]
	// Optional: clear the slot to avoid retaining references
	r.elements[r.head] = zero
	r.head = (r.head + 1) % len(r.elements)
	return value, true
}

// Peek returns the element at the head of the ring without removing it.
// It returns the zero value and false if the ring is empty.
func (r *RingBuffer[T]) Peek() (*T, bool) {
	if r.Len() == 0 {
		return nil, false
	}
	return &r.elements[r.head], true
}

// Discard discards the first N elements from the ring buffer.
// Returns the number of elements that are actually discarded (<= n). A
// non-positive N does not discard anything.
func (r *RingBuffer[T]) Discard(n int) int {
	if n <= 0 {
		return 0
	}
	currentLen := r.Len()
	n = min(n, currentLen)
	if n == currentLen {
		r.Clear()
		return n
	}
	cap := len(r.elements)
	end := r.head + n
	if end < cap {
		// no wrap: clear contiguous range
		clear(r.elements[r.head:end])
		r.head = end
	} else {
		// wraps around
		clear(r.elements[r.head:cap])
		clear(r.elements[:end-cap])
		r.head = end - cap
	}
	return n
}

// ForEach iterates over each element in the ring buffer,
// applying the provided function. If the function returns false,
// iteration stops early.
func (r *RingBuffer[T]) ForEach(fn func(*T) bool) {
	if r.Len() == 0 {
		return
	}
	if r.head < r.tail {
		// Contiguous data: [head ... tail)
		for i := r.head; i < r.tail; i++ {
			if !fn(&r.elements[i]) {
				return
			}
		}
	} else {
		// Wrapped data: [head ... end) + [0 ... tail)
		for i := r.head; i < len(r.elements); i++ {
			if !fn(&r.elements[i]) {
				return
			}
		}
		for i := 0; i < r.tail; i++ {
			if !fn(&r.elements[i]) {
				return
			}
		}
	}
}

// ForEachReverse iterates over each element in the ring buffer in reverse order,
// applying the provided function. If the function returns false,
// iteration stops early.
func (r *RingBuffer[T]) ForEachReverse(fn func(*T) bool) {
	if r.Len() == 0 {
		return
	}

	if r.head < r.tail {
		// Contiguous data: [head ... tail)
		for i := r.tail - 1; i >= r.head; i-- {
			if !fn(&r.elements[i]) {
				return
			}
		}
	} else {
		for i := r.tail - 1; i >= 0; i-- {
			if !fn(&r.elements[i]) {
				return
			}
		}
		for i := len(r.elements) - 1; i >= r.head; i-- {
			if !fn(&r.elements[i]) {
				return
			}
		}
	}
}

// Clear removes all elements and resets the ring to an empty state.
func (r *RingBuffer[T]) Clear() {
	var zero T
	// Only clear elements that contain data to avoid retaining references
	if r.head <= r.tail {
		for i := r.head; i < r.tail; i++ {
			r.elements[i] = zero
		}
	} else {
		for i := r.head; i < len(r.elements); i++ {
			r.elements[i] = zero
		}
		for i := 0; i < r.tail; i++ {
			r.elements[i] = zero
		}
	}
	r.head = 0
	r.tail = 0
}

// IsEmpty returns true if the ring has no elements.
func (r *RingBuffer[T]) IsEmpty() bool {
	return r.head == r.tail
}

// MaxLen returns the maximum number of elements the ring can hold. One backing
// slice slot is reserved to distinguish full from empty.
func (r *RingBuffer[T]) MaxLen() int {
	return len(r.elements) - 1
}

// IsFull returns true if advancing tail by one slot modulo the backing-slice
// length would make it equal to head.
func (r *RingBuffer[T]) IsFull() bool {
	return (r.tail+1)%len(r.elements) == r.head
}

// grow increases the ring buffer's backing-slice length when full.
// Growth policy:
//   - If the backing-slice length is below RINGBUFFER_MIN: grow to RINGBUFFER_MIN
//   - If it is below RINGBUFFER_EXP: double it
//   - If it is at or above RINGBUFFER_EXP: increase it by 10% (rounded up)
func (r *RingBuffer[T]) grow() {
	currentLength := r.Len()
	currentSize := len(r.elements)
	var newSize int

	switch {
	case currentSize < RINGBUFFER_MIN:
		newSize = RINGBUFFER_MIN
	case currentSize < RINGBUFFER_EXP:
		newSize = currentSize * 2
	default:
		newSize = currentSize + (currentSize+9)/10 // +10%, rounded up
	}

	newElements := make([]T, newSize)

	// Copy elements to new buffer preserving logical order
	if r.head < r.tail {
		// Contiguous data: [head ... tail)
		copy(newElements, r.elements[r.head:r.tail])
	} else {
		// Wrapped data: [head ... end) + [0 ... tail)
		n := copy(newElements, r.elements[r.head:])
		copy(newElements[n:], r.elements[:r.tail])
	}

	r.head = 0
	r.tail = currentLength
	r.elements = newElements
}

// At returns the i-th element from the head. It allows indexed iteration
// without a closure, which TinyGo would allocate on every call.
func (r *RingBuffer[T]) At(i int) *T {
	j := r.head + i
	if j >= len(r.elements) {
		j -= len(r.elements)
	}
	return &r.elements[j]
}
