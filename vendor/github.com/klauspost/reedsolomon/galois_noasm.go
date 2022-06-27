//go:build (!amd64 || noasm || appengine || gccgo) && (!arm64 || noasm || appengine || gccgo) && (!ppc64le || noasm || appengine || gccgo)
// +build !amd64 noasm appengine gccgo
// +build !arm64 noasm appengine gccgo
// +build !ppc64le noasm appengine gccgo

// Copyright 2015, Klaus Post, see LICENSE for details.

package reedsolomon

import "encoding/binary"

func galMulSlice(c byte, in, out []byte, o *options) {
	out = out[:len(in)]
	if c == 1 {
		copy(out, in)
		return
	}
	mt := mulTable[c][:256]
	for n, input := range in {
		out[n] = mt[input]
	}
}

func galMulSliceXor(c byte, in, out []byte, o *options) {
	out = out[:len(in)]
	if c == 1 {
		sliceXor(in, out, o)
		return
	}
	mt := mulTable[c][:256]
	for n, input := range in {
		out[n] ^= mt[input]
	}
}

// simple slice xor
func sliceXor(in, out []byte, _ *options) {
	for len(out) >= 32 {
		inS := in[:32]
		v0 := binary.LittleEndian.Uint64(out[:]) ^ binary.LittleEndian.Uint64(inS[:])
		v1 := binary.LittleEndian.Uint64(out[8:]) ^ binary.LittleEndian.Uint64(inS[8:])
		v2 := binary.LittleEndian.Uint64(out[16:]) ^ binary.LittleEndian.Uint64(inS[16:])
		v3 := binary.LittleEndian.Uint64(out[24:]) ^ binary.LittleEndian.Uint64(inS[24:])
		binary.LittleEndian.PutUint64(out[:], v0)
		binary.LittleEndian.PutUint64(out[8:], v1)
		binary.LittleEndian.PutUint64(out[16:], v2)
		binary.LittleEndian.PutUint64(out[24:], v3)
		out = out[32:]
		in = in[32:]
	}
	for n, input := range in {
		out[n] ^= input
	}
}

func init() {
	defaultOptions.useAVX512 = false
}
