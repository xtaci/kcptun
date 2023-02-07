//go:build noasm || nounsafe || gccgo || appengine

/**
 * Reed-Solomon Coding over 8-bit values.
 *
 * Copyright 2023, Klaus Post
 */

package reedsolomon

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
