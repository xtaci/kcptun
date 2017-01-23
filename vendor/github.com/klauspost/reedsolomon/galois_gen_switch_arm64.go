//go:build !appengine && !noasm && gc && !nogen && !nopshufb

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
	// ARM64 asm always reads 10 inputs; pad matrix and in-slice to this minimum.
	codeGenPadInputs = codeGenMaxInputs
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

func galMulSlicesSve(matrix []byte, in, out [][]byte, start, stop int) (n int) {
	n = stop - start

	if raceEnabled {
		defer func() {
			raceReadSlices(in, start, n)
			raceWriteSlices(out, start, n)
		}()
	}
	if len(in) < codeGenMaxInputs {
		var padded [codeGenMaxInputs][]byte
		copy(padded[:], in)
		for i := len(in); i < codeGenMaxInputs; i++ {
			padded[i] = in[0]
		}
		in = padded[:]
	}
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

func galMulSlicesSveXor(matrix []byte, in, out [][]byte, start, stop int) (n int) {
	n = (stop - start)

	if raceEnabled {
		defer func() {
			raceReadSlices(in, start, n)
			raceWriteSlices(out, start, n)
		}()
	}
	if len(in) < codeGenMaxInputs {
		var padded [codeGenMaxInputs][]byte
		copy(padded[:], in)
		for i := len(in); i < codeGenMaxInputs; i++ {
			padded[i] = in[0]
		}
		in = padded[:]
	}
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

// isFirstParityRowAllOne checks if the first parity row (first output) has all
// coefficients equal to 1 in the generator matrix.
//
// In Jerasure matrix, the first parity row is always all-ones, enabling XOR-only
// optimization for the first output. Subsequent parity rows typically have non-unity
// coefficients and require full GF multiplication.
//
// The matrix layout in memory is: (input * outputs + output_index) * 64
// This function checks only output_index=0 (first parity row).
func isFirstParityRowAllOne(matrix []byte, inputs, outputs int) bool {
	for i := 0; i < inputs; i++ {
		checkIndex := (i*outputs+0)*64 + 1
		if checkIndex >= len(matrix) {
			return false
		}
		if matrix[checkIndex] != 0x1 {
			return false
		}
	}
	return true
}

func galMulSlicesNeon(matrix []byte, in, out [][]byte, start, stop int) (n int) {
	n = stop - start
	if raceEnabled {
		defer func() {
			raceReadSlices(in, start, n)
			raceWriteSlices(out, start, n)
		}()
	}
	actualInputs := len(in)
	if len(in) < codeGenMaxInputs {
		var padded [codeGenMaxInputs][]byte
		copy(padded[:], in)
		for i := len(in); i < codeGenMaxInputs; i++ {
			padded[i] = in[0]
		}
		in = padded[:]
	}
	if 0 < len(out) && len(out) <= 4 && isFirstParityRowAllOne(matrix, actualInputs, len(out)) {
		eorIn := in[:actualInputs]
		switch len(out) {
		case 1:
			mulNeon_10x1_64eor(matrix, eorIn, out, start, n)
			return n & (maxInt - 63)
		case 2:
			mulNeon_10x2_64eor(matrix, eorIn, out, start, n)
			return n & (maxInt - 63)
		case 3:
			mulNeon_10x3_64eor(matrix, eorIn, out, start, n)
			return n & (maxInt - 63)
		case 4:
			mulNeon_10x4eor(matrix, eorIn, out, start, n)
			return n & (maxInt - 31)
		}
	}
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

func galMulSlicesNeonXor(matrix []byte, in, out [][]byte, start, stop int) (n int) {
	n = (stop - start)
	if raceEnabled {
		defer func() {
			raceReadSlices(in, start, n)
			raceWriteSlices(out, start, n)
		}()
	}
	if len(in) < codeGenMaxInputs {
		var padded [codeGenMaxInputs][]byte
		copy(padded[:], in)
		for i := len(in); i < codeGenMaxInputs; i++ {
			padded[i] = in[0]
		}
		in = padded[:]
	}
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
