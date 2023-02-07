//go:build !noasm && !nounsafe && !gccgo && !appengine

/**
 * Reed-Solomon Coding over 8-bit values.
 *
 * Copyright 2023, Klaus Post
 */

package reedsolomon

import (
	"unsafe"
)

// AllocAligned allocates 'shards' slices, with 'each' bytes.
// Each slice will start on a 64 byte aligned boundary.
func AllocAligned(shards, each int) [][]byte {
	if false {
		res := make([][]byte, shards)
		for i := range res {
			res[i] = make([]byte, each)
		}
		return res
	}
	const (
		alignEach  = 64
		alignStart = 64
	)
	eachAligned := ((each + alignEach - 1) / alignEach) * alignEach
	total := make([]byte, eachAligned*shards+63)
	align := uint(uintptr(unsafe.Pointer(&total[0]))) & (alignStart - 1)
	if align > 0 {
		total = total[alignStart-align:]
	}
	res := make([][]byte, shards)
	for i := range res {
		res[i] = total[:each:eachAligned]
		total = total[eachAligned:]
	}
	return res
}
