//+build !noasm
//+build !appengine
//+build !gccgo

// Copyright 2015, Klaus Post, see LICENSE for details.
// Copyright 2017, Minio, Inc.

package reedsolomon

//go:noescape
func galMulNEON(low, high, in, out []byte)

//go:noescape
func galMulXorNEON(low, high, in, out []byte)

//go:noescape
func galXorNEON(in, out []byte)

func galMulSlice(c byte, in, out []byte, o *options) {
	if c == 1 {
		copy(out, in)
		return
	}
	var done int
	galMulNEON(mulTableLow[c][:], mulTableHigh[c][:], in, out)
	done = (len(in) >> 5) << 5

	remain := len(in) - done
	if remain > 0 {
		mt := mulTable[c][:256]
		for i := done; i < len(in); i++ {
			out[i] = mt[in[i]]
		}
	}
}

func galMulSliceXor(c byte, in, out []byte, o *options) {
	if c == 1 {
		sliceXor(in, out, o)
		return
	}
	var done int
	galMulXorNEON(mulTableLow[c][:], mulTableHigh[c][:], in, out)
	done = (len(in) >> 5) << 5

	remain := len(in) - done
	if remain > 0 {
		mt := mulTable[c][:256]
		for i := done; i < len(in); i++ {
			out[i] ^= mt[in[i]]
		}
	}
}

// slice galois add
func sliceXor(in, out []byte, o *options) {

	galXorNEON(in, out)
	done := (len(in) >> 5) << 5

	remain := len(in) - done
	if remain > 0 {
		for i := done; i < len(in); i++ {
			out[i] ^= in[i]
		}
	}
}
