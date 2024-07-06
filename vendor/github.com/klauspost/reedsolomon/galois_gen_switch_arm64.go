//go:build !appengine && !noasm && gc && !nogen && !nopshufb
// +build !appengine,!noasm,gc,!nogen,!nopshufb

package reedsolomon

import (
	"fmt"
)

const (
	codeGen              = true
	codeGenMaxGoroutines = 16
	codeGenMaxInputs     = 10
	codeGenMaxOutputs    = 10
	minCodeGenSize       = 64
)

var (
	fSve     = galMulSlicesSve
	fSveXor  = galMulSlicesSveXor
	fNeon    = galMulSlicesNeon
	fNeonXor = galMulSlicesNeonXor
)

func (r *reedSolomon) hasCodeGen(byteCount int, inputs, outputs int) (_, _ *func(matrix []byte, in, out [][]byte, start, stop int) int, ok bool) {
	if r.o.useSVE {
		return &fSve, &fSveXor, codeGen && pshufb &&
			byteCount >= codeGenMinSize && inputs+outputs >= codeGenMinShards &&
			inputs <= codeGenMaxInputs && outputs <= codeGenMaxOutputs
	}
	return &fNeon, &fNeonXor, codeGen && pshufb && r.o.useNEON &&
		byteCount >= codeGenMinSize && inputs+outputs >= codeGenMinShards &&
		inputs <= codeGenMaxInputs && outputs <= codeGenMaxOutputs
}

func (r *reedSolomon) canGFNI(byteCount int, inputs, outputs int) (_, _ *func(matrix []uint64, in, out [][]byte, start, stop int) int, ok bool) {
	return nil, nil, false
}

// galMulSlicesSve
func galMulSlicesSve(matrix []byte, in, out [][]byte, start, stop int) int {
	n := stop - start

	// fmt.Println(len(in), len(out))
	switch len(out) {
	case 1:
		mulSve_10x1_64(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 2:
		mulSve_10x2_64(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 3:
		mulSve_10x3_64(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 4:
		mulSve_10x4(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 5:
		mulSve_10x5(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 6:
		mulSve_10x6(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 7:
		mulSve_10x7(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 8:
		mulSve_10x8(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 9:
		mulSve_10x9(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 10:
		mulSve_10x10(matrix, in, out, start, n)
		return n & (maxInt - 31)
	}
	panic(fmt.Sprintf("ARM SVE: unhandled size: %dx%d", len(in), len(out)))
}

// galMulSlicesSveXor
func galMulSlicesSveXor(matrix []byte, in, out [][]byte, start, stop int) int {
	n := (stop - start)

	switch len(out) {
	case 1:
		mulSve_10x1_64Xor(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 2:
		mulSve_10x2_64Xor(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 3:
		mulSve_10x3_64Xor(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 4:
		mulSve_10x4Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 5:
		mulSve_10x5Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 6:
		mulSve_10x6Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 7:
		mulSve_10x7Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 8:
		mulSve_10x8Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 9:
		mulSve_10x9Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 10:
		mulSve_10x10Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	}
	panic(fmt.Sprintf("ARM SVE: unhandled size: %dx%d", len(in), len(out)))
}

// galMulSlicesNeon
func galMulSlicesNeon(matrix []byte, in, out [][]byte, start, stop int) int {
	n := stop - start

	switch len(out) {
	case 1:
		mulNeon_10x1_64(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 2:
		mulNeon_10x2_64(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 3:
		mulNeon_10x3_64(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 4:
		mulNeon_10x4(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 5:
		mulNeon_10x5(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 6:
		mulNeon_10x6(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 7:
		mulNeon_10x7(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 8:
		mulNeon_10x8(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 9:
		mulNeon_10x9(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 10:
		mulNeon_10x10(matrix, in, out, start, n)
		return n & (maxInt - 31)
	}
	panic(fmt.Sprintf("ARM NEON: unhandled size: %dx%d", len(in), len(out)))
}

// galMulSlicesNeonXor
func galMulSlicesNeonXor(matrix []byte, in, out [][]byte, start, stop int) int {
	n := (stop - start)

	switch len(out) {
	case 1:
		mulNeon_10x1_64Xor(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 2:
		mulNeon_10x2_64Xor(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 3:
		mulNeon_10x3_64Xor(matrix, in, out, start, n)
		return n & (maxInt - 63)
	case 4:
		mulNeon_10x4Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 5:
		mulNeon_10x5Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 6:
		mulNeon_10x6Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 7:
		mulNeon_10x7Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 8:
		mulNeon_10x8Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 9:
		mulNeon_10x9Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	case 10:
		mulNeon_10x10Xor(matrix, in, out, start, n)
		return n & (maxInt - 31)
	}
	panic(fmt.Sprintf("ARM NEON: unhandled size: %dx%d", len(in), len(out)))
}
