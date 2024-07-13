package qpp

// xorshift64star is a pseudo-random number generator that is part of the xorshift family of PRNGs.
func xorshift64star(state uint64) uint64 {
	state ^= state >> 12
	state ^= state << 25
	state ^= state >> 27
	return state * 2685821657736338717
}

// xorshift32
func xorshift32(state uint32) uint32 {
	state ^= state << 13
	state ^= state >> 17
	state ^= state << 5
	return state
}

// xorshift16
//
//go:inline
func xorshift16(state uint16) uint16 {
	state ^= state << 7
	state ^= state >> 9
	state ^= state << 8
	return state
}
