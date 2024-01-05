//go:build (!amd64 || noasm || appengine || gccgo) && (!arm64 || noasm || appengine || gccgo || nopshufb) && (!ppc64le || noasm || appengine || gccgo || nopshufb)

// Copyright 2015, Klaus Post, see LICENSE for details.

package reedsolomon

const pshufb = false

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
	sliceXorGo(x, y, o)
}

// 2-way butterfly forward
func fftDIT28(x, y []byte, log_m ffe8, o *options) {
	// Reference version:
	refMulAdd8(x, y, log_m)
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
	refMulAdd8(x, y, log_m)
}

func mulgf16(x, y []byte, log_m ffe, o *options) {
	refMul(x, y, log_m)
}

func mulgf8(x, y []byte, log_m ffe8, o *options) {
	refMul8(x, y, log_m)
}
