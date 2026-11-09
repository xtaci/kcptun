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
	"errors"
	"sync"
)

var (
	defaultAllocator *Allocator
	debruijnPos      = [...]byte{0, 9, 1, 10, 13, 21, 2, 29, 11, 14, 16, 18, 22, 25, 3, 30, 8, 12, 20, 28, 15, 17, 24, 7, 19, 27, 23, 6, 26, 5, 4, 31}
)

func init() {
	defaultAllocator = NewAllocator()
}

// Allocator pools byte slices for incoming frames.
type Allocator struct {
	buffers []sync.Pool
}

// NewAllocator initializes a byte-slice allocator for frames up to 65536 bytes.
// Each allocation wastes less than half of its capacity.
func NewAllocator() *Allocator {
	alloc := new(Allocator)
	alloc.buffers = make([]sync.Pool, 17) // 1B -> 64K
	for k := range alloc.buffers {
		i := k
		alloc.buffers[k].New = func() any {
			b := make([]byte, 1<<uint32(i))
			return &b
		}
	}
	return alloc
}

// Get returns a byte slice with enough capacity for size, or nil if size is invalid.
func (alloc *Allocator) Get(size int) *[]byte {
	if size <= 0 || size > 65536 {
		return nil
	}

	bits := msb(size)
	if size == 1<<bits {
		p := alloc.buffers[bits].Get().(*[]byte)
		*p = (*p)[:size]
		return p
	}
	p := alloc.buffers[bits+1].Get().(*[]byte)
	*p = (*p)[:size]
	return p
}

// Put returns a byte slice to the pool. Its capacity must be a power of two.
func (alloc *Allocator) Put(p *[]byte) error {
	if p == nil {
		return errors.New("allocator Put() incorrect buffer size")
	}
	bits := msb(cap(*p))
	if cap(*p) == 0 || cap(*p) > 65536 || cap(*p) != 1<<bits {
		return errors.New("allocator Put() incorrect buffer size")
	}
	alloc.buffers[bits].Put(p)
	return nil
}

// msb returns the position of the most significant set bit.
// http://supertech.csail.mit.edu/papers/debruijn.pdf
func msb(size int) byte {
	v := uint32(size)
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	return debruijnPos[(v*0x07C4ACDD)>>27]
}
