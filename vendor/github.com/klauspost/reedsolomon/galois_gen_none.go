//go:build !amd64 || noasm || appengine || gccgo || nogen
// +build !amd64 noasm appengine gccgo nogen

package reedsolomon

const maxAvx2Inputs = 1
const maxAvx2Outputs = 1
const minAvx2Size = 1
const avxSizeMask = 0
const avx2CodeGen = false

func galMulSlicesAvx2(matrix []byte, in, out [][]byte, start, stop int) int {
	panic("avx2 codegen not available")
}

func galMulSlicesAvx2Xor(matrix []byte, in, out [][]byte, start, stop int) int {
	panic("avx2 codegen not available")
}
