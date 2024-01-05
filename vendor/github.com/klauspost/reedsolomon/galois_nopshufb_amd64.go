// Copyright 2015, Klaus Post, see LICENSE for details

//go:build nopshufb && !noasm

package reedsolomon

// bigSwitchover is the size where 64 bytes are processed per loop.
const bigSwitchover = 128

const pshufb = false

// simple slice xor
func sliceXor(in, out []byte, o *options) {
	if o.useSSE2 {
		if len(in) >= bigSwitchover {
			if o.useAVX2 {
				avx2XorSlice_64(in, out)
				done := (len(in) >> 6) << 6
				in = in[done:]
				out = out[done:]
			} else {
				sSE2XorSlice_64(in, out)
				done := (len(in) >> 6) << 6
				in = in[done:]
				out = out[done:]
			}
		}
		if len(in) >= 16 {
			sSE2XorSlice(in, out)
			done := (len(in) >> 4) << 4
			in = in[done:]
			out = out[done:]
		}
	} else {
		sliceXorGo(in, out, o)
		return
	}
	out = out[:len(in)]
	for i := range in {
		out[i] ^= in[i]
	}
}

func galMulSlice(c byte, in, out []byte, o *options) {
	out = out[:len(in)]
	if c == 1 {
		copy(out, in)
		return
	}
	mt := mulTable[c][:256]
	for len(in) >= 4 {
		ii := (*[4]byte)(in)
		oo := (*[4]byte)(out)
		oo[0] = mt[ii[0]]
		oo[1] = mt[ii[1]]
		oo[2] = mt[ii[2]]
		oo[3] = mt[ii[3]]
		in = in[4:]
		out = out[4:]
	}
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
	for len(in) >= 4 {
		ii := (*[4]byte)(in)
		oo := (*[4]byte)(out)
		oo[0] ^= mt[ii[0]]
		oo[1] ^= mt[ii[1]]
		oo[2] ^= mt[ii[2]]
		oo[3] ^= mt[ii[3]]
		in = in[4:]
		out = out[4:]
	}
	for n, input := range in {
		out[n] ^= mt[input]
	}
}

func init() {
	defaultOptions.useAVX512 = false
}

// 4-way butterfly
func ifftDIT4(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	ifftDIT4Ref(work, dist, log_m01, log_m23, log_m02, o)
}

// 4-way butterfly
func ifftDIT48(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	ifftDIT4Ref8(work, dist, log_m01, log_m23, log_m02, o)
}

// 4-way butterfly
func fftDIT4(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	fftDIT4Ref(work, dist, log_m01, log_m23, log_m02, o)
}

// 4-way butterfly
func fftDIT48(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	fftDIT4Ref8(work, dist, log_m01, log_m23, log_m02, o)
}

// 2-way butterfly forward
func fftDIT2(x, y []byte, log_m ffe, o *options) {
	// Reference version:
	refMulAdd(x, y, log_m)
	sliceXor(x, y, o)
}

// 2-way butterfly forward
func fftDIT28(x, y []byte, log_m ffe8, o *options) {
	// Reference version:
	refMulAdd8(x, y, log_m)
	sliceXor(x, y, o)
}

// 2-way butterfly inverse
func ifftDIT2(x, y []byte, log_m ffe, o *options) {
	// Reference version:
	sliceXor(x, y, o)
	refMulAdd(x, y, log_m)
}

// 2-way butterfly inverse
func ifftDIT28(x, y []byte, log_m ffe8, o *options) {
	// Reference version:
	sliceXor(x, y, o)
	refMulAdd8(x, y, log_m)
}

func mulgf16(x, y []byte, log_m ffe, o *options) {
	refMul(x, y, log_m)
}

func mulgf8(x, y []byte, log_m ffe8, o *options) {
	refMul8(x, y, log_m)
}
