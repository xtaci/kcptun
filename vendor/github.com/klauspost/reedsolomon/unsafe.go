//go:build !nounsafe && !gccgo && !appengine

/**
 * Reed-Solomon Coding over 8-bit values.
 *
 * Copyright 2023, Klaus Post
 */

package reedsolomon

import (
	"unsafe"
)

const unsafeEnabled = true

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

// load64 will load from b at index i.
func load64[I indexer](b []byte, i I) uint64 {
	//return binary.LittleEndian.Uint64(b[i:])
	//return *(*uint64)(unsafe.Pointer(&b[i]))
	return *(*uint64)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(b)), i))
}

// Store64 will store v at b.
func store64[I indexer](b []byte, v uint64, i I) {
	//binary.LittleEndian.PutUint64(b[i:], v)
	*(*uint64)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(b)), i)) = v
}

// load16 will load from b at index i.
func load16[I indexer](b []byte, i I) uint16 {
	//return binary.LittleEndian.Uint64(b[i:])
	//return *(*uint64)(unsafe.Pointer(&b[i]))
	return *(*uint16)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(b)), i))
}

// Store16 will store v at b.
func store16[I indexer](b []byte, v uint16, i I) {
	//binary.LittleEndian.PutUint64(b[i:], v)
	*(*uint16)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(b)), i)) = v
}

func load8[I indexer](b []byte, i I) uint8 {
	return *(*uint8)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(b)), i))
}

func store8[I indexer](b []byte, v uint8, i I) {
	*(*uint8)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(b)), i)) = v
}
