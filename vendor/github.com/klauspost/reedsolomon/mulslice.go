package reedsolomon

import (
	"cmp"
	"fmt"
)

// LowLevel exposes low level functionality.
type LowLevel struct {
	o *options
}

// WithOptions resets the options to the default+provided options.
// Options that don't apply to the called functions will be ignored.
// This should not be called concurrent with other calls.
func (l *LowLevel) WithOptions(opts ...Option) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}
	l.o = &o
}

func (l LowLevel) options() *options {
	return cmp.Or(l.o, &defaultOptions)
}

// GalMulSlice multiplies the elements of in by c, writing the result to out: out[i] = c * in[i].
// out must be at least as long as in.
func (l LowLevel) GalMulSlice(c byte, in, out []byte) {
	if len(out) < len(in) {
		panic(fmt.Sprintf("reedsolomon: GalMulSlice: len(out)=%d, must be >= len(in)=%d", len(out), len(in)))
	}
	galMulSlice(c, in, out, l.options())
}

// GalMulSliceXor multiplies the elements of in by c, and adds the result to out: out[i] ^= c * in[i].
// out must be at least as long as in.
func (l LowLevel) GalMulSliceXor(c byte, in, out []byte) {
	if len(out) < len(in) {
		panic(fmt.Sprintf("reedsolomon: GalMulSliceXor: len(out)=%d, must be >= len(in)=%d", len(out), len(in)))
	}
	galMulSliceXor(c, in, out, l.options())
}

// Inv returns the multiplicative inverse of e in GF(2^8).
// Should not be called with 0 (returns 0 in this case).
func Inv(e byte) byte {
	return invTable[e]
}

// GF16MulSliceXor8 fuses 8 scalar-broadcast GF(2^16) mul-XOR accumulates
// that share the same input slice `in`:
//
//	out_k[i] ^= scalars[k] * in[i]   for k in [0, 8)
//
// All 8 destination slices must share length with in, which must be a
// multiple of 64 bytes (Leopard's chunk size: 32 low bytes + 32 high bytes
// per chunk). A zero-length input is valid and returns immediately.
// A zero scalar contributes nothing; callers do not need to filter them.
func (l LowLevel) GF16MulSliceXor8(scalars *[8]uint16, in []byte, outs *[8][]byte) {
	if len(in) == 0 {
		return
	}
	if len(in)%64 != 0 {
		panic(fmt.Sprintf("reedsolomon: GF16MulSliceXor8: len(in)=%d must be a multiple of 64", len(in)))
	}
	for k := range outs {
		if len(outs[k]) != len(in) {
			panic(fmt.Sprintf("reedsolomon: GF16MulSliceXor8: outs[%d] has len %d, expected %d", k, len(outs[k]), len(in)))
		}
	}
	initConstants()
	mulgf16Xor8(scalars, in, outs, l.options())
}

// GF16MulSliceXor multiplies each GF(2^16) element of in by scalar and
// XOR-accumulates the result into out:
//
//	out[i] ^= scalar * in[i]
//
// Both slices must have equal length, which must be a multiple of 64 bytes
// (Leopard's chunk size: 32 low bytes + 32 high bytes per chunk).
// A zero-length input is valid and returns immediately.
// A zero scalar is a no-op.
func (l LowLevel) GF16MulSliceXor(scalar uint16, in []byte, out []byte) {
	if len(in) == 0 {
		return
	}
	if scalar == 0 {
		return
	}
	if len(in)%64 != 0 {
		panic(fmt.Sprintf("reedsolomon: GF16MulSliceXor: len(in)=%d must be a multiple of 64", len(in)))
	}
	if len(out) != len(in) {
		panic(fmt.Sprintf("reedsolomon: GF16MulSliceXor: len(out)=%d, expected %d", len(out), len(in)))
	}
	initConstants()
	o := l.options()
	mulgf16Xor(out, in, logLUT[ffe(scalar)], o)
}
