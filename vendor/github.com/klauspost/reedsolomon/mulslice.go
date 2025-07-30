package reedsolomon

import "cmp"

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
}

func (l LowLevel) options() *options {
	return cmp.Or(l.o, &defaultOptions)
}

// GalMulSlice multiplies the elements of in by c, writing the result to out: out[i] = c * in[i].
// out must be at least as long as in.
func (l LowLevel) GalMulSlice(c byte, in, out []byte) {
	galMulSlice(c, in, out, l.options())
}

// GalMulSliceXor multiplies the elements of in by c, and adds the result to out: out[i] ^= c * in[i].
// out must be at least as long as in.
func (l LowLevel) GalMulSliceXor(c byte, in, out []byte) {
	galMulSliceXor(c, in, out, l.options())
}

// Inv returns the multiplicative inverse of e in GF(2^8).
// Should not be called with 0 (returns 0 in this case).
func Inv(e byte) byte {
	return invTable[e]
}
