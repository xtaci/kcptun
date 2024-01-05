//go:build !amd64 || noasm || appengine || gccgo || pshufb

// Copyright 2020, Klaus Post, see LICENSE for details.

package reedsolomon

func (r *reedSolomon) codeSomeShardsAvx512(matrixRows, inputs, outputs [][]byte, byteCount int) {
	panic("codeSomeShardsAvx512 should not be called if built without asm")
}

func (r *reedSolomon) codeSomeShardsAvx512P(matrixRows, inputs, outputs [][]byte, byteCount int) {
	panic("codeSomeShardsAvx512P should not be called if built without asm")
}
