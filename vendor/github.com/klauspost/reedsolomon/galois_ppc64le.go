//go:build !noasm && !appengine && !gccgo && !nopshufb

// Copyright 2015, Klaus Post, see LICENSE for details.
// Copyright 2018, Minio, Inc.

package reedsolomon

const pshufb = true

//go:noescape
func galMulPpc(low, high, in, out []byte)

//go:noescape
func galMulPpcXor(low, high, in, out []byte)

// This is what the assembler routines do in blocks of 16 bytes:
/*
func galMulPpc(low, high, in, out []byte) {
	for n, input := range in {
		l := input & 0xf
		h := input >> 4
		out[n] = low[l] ^ high[h]
	}
}
func galMulPpcXor(low, high, in, out []byte) {
	for n, input := range in {
		l := input & 0xf
		h := input >> 4
		out[n] ^= low[l] ^ high[h]
	}
}
*/

func galMulSlice(c byte, in, out []byte, o *options) {
	if c == 1 {
		copy(out, in)
		return
	}
	done := (len(in) >> 4) << 4
	if done > 0 {
		galMulPpc(mulTableLow[c][:], mulTableHigh[c][:], in[:done], out)
	}
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
	done := (len(in) >> 4) << 4
	if done > 0 {
		galMulPpcXor(mulTableLow[c][:], mulTableHigh[c][:], in[:done], out)
	}
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
	sliceXorGo(x, y, o)
}

// 2-way butterfly forward
func fftDIT28(x, y []byte, log_m ffe8, o *options) {
	// Reference version:
	mulAdd8(x, y, log_m, o)
	sliceXorGo(x, y, o)
}

// 2-way butterfly inverse
func ifftDIT2(x, y []byte, log_m ffe, o *options) {
	// Reference version:
	sliceXorGo(x, y, o)
	refMulAdd(x, y, log_m)
}

// 2-way butterfly inverse
func ifftDIT28(x, y []byte, log_m ffe8, o *options) {
	// Reference version:
	sliceXorGo(x, y, o)
	mulAdd8(x, y, log_m, o)
}

func mulgf16(x, y []byte, log_m ffe, o *options) {
	refMul(x, y, log_m)
}

func mulAdd8(out, in []byte, log_m ffe8, o *options) {
	t := &multiply256LUT8[log_m]
	galMulPpcXor(t[:16], t[16:32], in, out)
	done := (len(in) >> 4) << 4
	in = in[done:]
	if len(in) > 0 {
		out = out[done:]
		refMulAdd8(in, out, log_m)
	}
}

func mulgf8(out, in []byte, log_m ffe8, o *options) {
	var done int
	t := &multiply256LUT8[log_m]
	galMulPpc(t[:16], t[16:32], in, out)
	done = (len(in) >> 4) << 4

	remain := len(in) - done
	if remain > 0 {
		mt := mul8LUTs[log_m].Value[:]
		for i := done; i < len(in); i++ {
			out[i] ^= byte(mt[in[i]])
		}
	}
}
