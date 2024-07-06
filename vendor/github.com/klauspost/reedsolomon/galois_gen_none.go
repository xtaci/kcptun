//go:build !(amd64 || arm64) || noasm || appengine || gccgo || nogen

package reedsolomon

const (
	codeGen              = false
	codeGenMaxGoroutines = 8
	codeGenMaxInputs     = 1
	codeGenMaxOutputs    = 1
	minCodeGenSize       = 1
)

func (r *reedSolomon) hasCodeGen(int, int, int) (_, _ *func(matrix []byte, in, out [][]byte, start, stop int) int, ok bool) {
	return nil, nil, false
}

func (r *reedSolomon) canGFNI(int, int, int) (_, _ *func(matrix []uint64, in, out [][]byte, start, stop int) int, ok bool) {
	return nil, nil, false
}
