package reedsolomon

import (
	"fmt"
	"sync"
)

// WorkAllocator provides external work buffer management for the leopard
// Reed-Solomon encoder (both GF(2^8) and GF(2^16) variants). When set via
// [WithWorkAllocator], it replaces the internal sync.Pool used by the leopard
// encoder for temporary work buffers during Encode and Reconstruct.
//
// This only affects the leopard-based encoder ([WithLeopardGF16], [WithLeopardGF]).
// The classic matrix-based encoder does not use work buffers and ignores this option.
//
// This is useful when the caller wants to control buffer lifecycle (e.g., to
// avoid sync.Pool churn under GC pressure with large buffers).
//
// Implementations must be safe for concurrent use: Get and Put may be
// called from multiple goroutines when the encoder is shared.
//
// Get must return n byte slices, each with len and cap >= size.
// Slices should be SIMD-aligned (64-byte) for optimal performance; see [AllocAligned].
// Put is called when the work buffer is no longer needed.
type WorkAllocator interface {
	Get(n, size int) [][]byte
	Put([][]byte)
}

// getWork returns n buffers of at least size bytes from a.
// The buffers go straight to the SIMD kernels, which do not re-check lengths,
// so a WorkAllocator breaking its contract is reported instead of corrupting memory.
func getWork(a WorkAllocator, n, size int) ([][]byte, error) {
	work := a.Get(n, size)
	if len(work) < n {
		a.Put(work)
		return nil, fmt.Errorf("%w: WorkAllocator returned %d buffers, need %d", ErrInvalidInput, len(work), n)
	}
	work = work[:n]
	for i, w := range work {
		if len(w) < size {
			a.Put(work)
			return nil, fmt.Errorf("%w: WorkAllocator buffer %d has %d bytes, need %d", ErrInvalidInput, i, len(w), size)
		}
	}
	return work, nil
}

// defaultWorkAllocator is the default WorkAllocator backed by sync.Pool.
type defaultWorkAllocator struct {
	pool sync.Pool
}

func (a *defaultWorkAllocator) Get(n, size int) [][]byte {
	var work [][]byte
	if w, ok := a.pool.Get().([][]byte); ok {
		work = w
	}
	if cap(work) >= n {
		work = work[:n]
	} else {
		work = AllocAligned(n, size)
	}
	for i := range work {
		if cap(work[i]) < size {
			work[i] = AllocAligned(1, size)[0]
		} else {
			work[i] = work[i][:size]
		}
	}
	return work
}

func (a *defaultWorkAllocator) Put(work [][]byte) {
	a.pool.Put(work)
}
