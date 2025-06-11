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
	RINGBUFFER_MIN = 8
	RINGBUFFER_EXP = 1024
)

// RingBuffer is a generic ring (circular) buffer that supports dynamic resizing.
// It provides efficient FIFO queue behavior with amortized constant time operations.
type RingBuffer[T any] struct {
	head     int // Index of the next element to be popped
	tail     int // Index of the next empty slot to push into
	elements []T // Underlying slice storing elements in circular fashion
}

// NewRingBuffer creates a new Ring with a specified initial capacity.
// If the provided size is <= 8, it defaults to 8.
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

	return len(r.elements[r.head:]) + len(r.elements[:r.tail])
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
// Returns the number of elements that are actually discarded (<= n).
func (r *RingBuffer[T]) Discard(n int) int {
	n = min(n, r.Len())
	if n == r.Len() {
		r.Clear()
		return n
	}
	var zero T
	for range n {
		r.elements[r.head] = zero
		r.head = (r.head + 1) % len(r.elements)
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

// Clear resets the ring to an empty state and reinitializes the buffer.
func (r *RingBuffer[T]) Clear() {
	r.head = 0
	r.tail = 0

	var zero T
	for i := range r.elements {
		r.elements[i] = zero // Clear each slot to avoid retaining references
	}
}

// IsEmpty returns true if the ring has no elements.
func (r *RingBuffer[T]) IsEmpty() bool {
	return r.Len() == 0
}

// MaxLen returns the maximum capacity of the ring buffer.
func (r *RingBuffer[T]) MaxLen() int {
	return len(r.elements) - 1
}

// IsFull returns true if the ring buffer is full (tail + 1 == head).
func (r *RingBuffer[T]) IsFull() bool {
	return (r.tail+1)%len(r.elements) == r.head
}

// grow increases the ring buffer's capacity when full.
// Growth policy:
//   - If current size < 8: grow to 8
//   - If size <= 4096: double the size
//   - If size > 4096: increase by 10% (rounded up)
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
