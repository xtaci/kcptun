//+build !amd64 noasm appengine gccgo
//+build !arm64 noasm appengine gccgo
//+build !ppc64le noasm appengine gccgo

// Copyright 2015, Klaus Post, see LICENSE for details.

package reedsolomon

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
		for n, input := range in {
			out[n] ^= input
		}
		return
	}
	mt := mulTable[c][:256]
	for n, input := range in {
		out[n] ^= mt[input]
	}
}

// slice galois add
func sliceXor(in, out []byte, o *options) {
	for n, input := range in {
		out[n] ^= input
	}
}

func init() {
	defaultOptions.useAVX512 = false
}
