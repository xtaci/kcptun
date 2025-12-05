//go:build nounsafe || gccgo || appengine

/**
 * Reed-Solomon Coding over 8-bit values.
 *
 * Copyright 2023, Klaus Post
 */

package reedsolomon

import "encoding/binary"

const unsafeEnabled = false

// AllocAligned allocates 'shards' slices, with 'each' bytes.
// Each slice will start on a 64 byte aligned boundary.
func AllocAligned(shards, each int) [][]byte {
	eachAligned := ((each + 63) / 64) * 64
	total := make([]byte, eachAligned*shards+63)
	// We cannot do initial align without "unsafe", just use native alignment.
	res := make([][]byte, shards)
	for i := range res {
		res[i] = total[:each:eachAligned]
		total = total[eachAligned:]
	}
	return res
}

// load64 will load from b at index i.
func load64[I indexer](b []byte, i I) uint64 {
	return binary.LittleEndian.Uint64(b[i:])
}

// Store64 will store v at b.
func store64[I indexer](b []byte, v uint64, i I) {
	binary.LittleEndian.PutUint64(b[i:], v)
}

// load8 will load from b at index i.
func load8[I indexer](b []byte, i I) byte {
	return b[i]
}

// load16 will load from b at index i.
func load16[I indexer](b []byte, i I) uint16 {
	return binary.LittleEndian.Uint16(b[i:])
}

func store16[I indexer](b []byte, v uint16, i I) {
	binary.LittleEndian.PutUint16(b[i:], v)
}

func store8[I indexer](b []byte, v byte, i I) {
	b[i] = v
}
