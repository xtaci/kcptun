//+build !noasm
//+build !appengine
//+build !gccgo

// Copyright 2015, Klaus Post, see LICENSE for details.

package reedsolomon

//go:noescape
func galMulSSSE3(low, high, in, out []byte)

//go:noescape
func galMulSSSE3Xor(low, high, in, out []byte)

//go:noescape
func galMulAVX2Xor(low, high, in, out []byte)

//go:noescape
func galMulAVX2(low, high, in, out []byte)

//go:noescape
func sSE2XorSlice(in, out []byte)

//go:noescape
func galMulAVX2Xor_64(low, high, in, out []byte)

//go:noescape
func galMulAVX2_64(low, high, in, out []byte)

//go:noescape
func sSE2XorSlice_64(in, out []byte)

// This is what the assembler routines do in blocks of 16 bytes:
/*
func galMulSSSE3(low, high, in, out []byte) {
	for n, input := range in {
		l := input & 0xf
		h := input >> 4
		out[n] = low[l] ^ high[h]
	}
}

func galMulSSSE3Xor(low, high, in, out []byte) {
	for n, input := range in {
		l := input & 0xf
		h := input >> 4
		out[n] ^= low[l] ^ high[h]
	}
}
*/

// bigSwitchover is the size where 64 bytes are processed per loop.
const bigSwitchover = 128

func galMulSlice(c byte, in, out []byte, o *options) {
	if c == 1 {
		copy(out, in)
		return
	}
	if o.useAVX2 {
		if len(in) >= bigSwitchover {
			galMulAVX2_64(mulTableLow[c][:], mulTableHigh[c][:], in, out)
			done := (len(in) >> 6) << 6
			in = in[done:]
			out = out[done:]
		}
		if len(in) > 32 {
			galMulAVX2(mulTableLow[c][:], mulTableHigh[c][:], in, out)
			done := (len(in) >> 5) << 5
			in = in[done:]
			out = out[done:]
		}
	} else if o.useSSSE3 {
		galMulSSSE3(mulTableLow[c][:], mulTableHigh[c][:], in, out)
		done := (len(in) >> 4) << 4
		in = in[done:]
		out = out[done:]
	}
	out = out[:len(in)]
	mt := mulTable[c][:256]
	for i := range in {
		out[i] = mt[in[i]]
	}
}

func galMulSliceXor(c byte, in, out []byte, o *options) {
	if c == 1 {
		sliceXor(in, out, o)
		return
	}

	if o.useAVX2 {
		if len(in) >= bigSwitchover {
			galMulAVX2Xor_64(mulTableLow[c][:], mulTableHigh[c][:], in, out)
			done := (len(in) >> 6) << 6
			in = in[done:]
			out = out[done:]
		}
		if len(in) >= 32 {
			galMulAVX2Xor(mulTableLow[c][:], mulTableHigh[c][:], in, out)
			done := (len(in) >> 5) << 5
			in = in[done:]
			out = out[done:]
		}
	} else if o.useSSSE3 {
		galMulSSSE3Xor(mulTableLow[c][:], mulTableHigh[c][:], in, out)
		done := (len(in) >> 4) << 4
		in = in[done:]
		out = out[done:]
	}
	out = out[:len(in)]
	mt := mulTable[c][:256]
	for i := range in {
		out[i] ^= mt[in[i]]
	}
}

// slice galois add
func sliceXor(in, out []byte, o *options) {
	if o.useSSE2 {
		if len(in) >= bigSwitchover {
			sSE2XorSlice_64(in, out)
			done := (len(in) >> 6) << 6
			in = in[done:]
			out = out[done:]
		}
		if len(in) >= 16 {
			sSE2XorSlice(in, out)
			done := (len(in) >> 4) << 4
			in = in[done:]
			out = out[done:]
		}
	}
	out = out[:len(in)]
	for i := range in {
		out[i] ^= in[i]
	}
}
