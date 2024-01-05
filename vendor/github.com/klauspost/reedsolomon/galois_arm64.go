//go:build !noasm && !appengine && !gccgo && !nopshufb

// Copyright 2015, Klaus Post, see LICENSE for details.
// Copyright 2017, Minio, Inc.

package reedsolomon

const pshufb = true

//go:noescape
func galMulNEON(low, high, in, out []byte)

//go:noescape
func galMulXorNEON(low, high, in, out []byte)

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
	// 64 byte aligned, always full.
	xorSliceNEON(x, y)
}

// 2-way butterfly forward
func fftDIT28(x, y []byte, log_m ffe8, o *options) {
	// Reference version:
	mulAdd8(x, y, log_m, o)
	sliceXor(x, y, o)
}

// 2-way butterfly
func ifftDIT2(x, y []byte, log_m ffe, o *options) {
	// 64 byte aligned, always full.
	xorSliceNEON(x, y)
	// Reference version:
	refMulAdd(x, y, log_m)
}

// 2-way butterfly inverse
func ifftDIT28(x, y []byte, log_m ffe8, o *options) {
	// Reference version:
	sliceXor(x, y, o)
	mulAdd8(x, y, log_m, o)
}

func mulgf16(x, y []byte, log_m ffe, o *options) {
	refMul(x, y, log_m)
}

func mulAdd8(out, in []byte, log_m ffe8, o *options) {
	t := &multiply256LUT8[log_m]
	galMulXorNEON(t[:16], t[16:32], in, out)
	done := (len(in) >> 5) << 5
	in = in[done:]
	if len(in) > 0 {
		out = out[done:]
		refMulAdd8(in, out, log_m)
	}
}

func mulgf8(out, in []byte, log_m ffe8, o *options) {
	var done int
	t := &multiply256LUT8[log_m]
	galMulNEON(t[:16], t[16:32], in, out)
	done = (len(in) >> 5) << 5

	remain := len(in) - done
	if remain > 0 {
		mt := mul8LUTs[log_m].Value[:]
		for i := done; i < len(in); i++ {
			out[i] ^= byte(mt[in[i]])
		}
	}
}
