//go:build !noasm && !appengine && !gccgo
// +build !noasm,!appengine,!gccgo

// Copyright 2015, Klaus Post, see LICENSE for details.
// Copyright 2019, Minio, Inc.

package reedsolomon

import (
	"sync"
)

//go:noescape
func _galMulAVX512Parallel81(in, out [][]byte, matrix *[matrixSize81]byte, addTo bool)

//go:noescape
func _galMulAVX512Parallel82(in, out [][]byte, matrix *[matrixSize82]byte, addTo bool)

//go:noescape
func _galMulAVX512Parallel84(in, out [][]byte, matrix *[matrixSize84]byte, addTo bool)

const (
	dimIn        = 8                            // Number of input rows processed simultaneously
	dimOut81     = 1                            // Number of output rows processed simultaneously for x1 routine
	dimOut82     = 2                            // Number of output rows processed simultaneously for x2 routine
	dimOut84     = 4                            // Number of output rows processed simultaneously for x4 routine
	matrixSize81 = (16 + 16) * dimIn * dimOut81 // Dimension of slice of matrix coefficient passed into x1 routine
	matrixSize82 = (16 + 16) * dimIn * dimOut82 // Dimension of slice of matrix coefficient passed into x2 routine
	matrixSize84 = (16 + 16) * dimIn * dimOut84 // Dimension of slice of matrix coefficient passed into x4 routine
)

// Construct block of matrix coefficients for single output row in parallel
func setupMatrix81(matrixRows [][]byte, inputOffset, outputOffset int, matrix *[matrixSize81]byte) {
	offset := 0
	for c := inputOffset; c < inputOffset+dimIn; c++ {
		for iRow := outputOffset; iRow < outputOffset+dimOut81; iRow++ {
			if c < len(matrixRows[iRow]) {
				coeff := matrixRows[iRow][c]
				copy(matrix[offset*32:], mulTableLow[coeff][:])
				copy(matrix[offset*32+16:], mulTableHigh[coeff][:])
			} else {
				// coefficients not used for this input shard (so null out)
				v := matrix[offset*32 : offset*32+32]
				for i := range v {
					v[i] = 0
				}
			}
			offset += dimIn
			if offset >= dimIn*dimOut81 {
				offset -= dimIn*dimOut81 - 1
			}
		}
	}
}

// Construct block of matrix coefficients for 2 output rows in parallel
func setupMatrix82(matrixRows [][]byte, inputOffset, outputOffset int, matrix *[matrixSize82]byte) {
	offset := 0
	for c := inputOffset; c < inputOffset+dimIn; c++ {
		for iRow := outputOffset; iRow < outputOffset+dimOut82; iRow++ {
			if c < len(matrixRows[iRow]) {
				coeff := matrixRows[iRow][c]
				copy(matrix[offset*32:], mulTableLow[coeff][:])
				copy(matrix[offset*32+16:], mulTableHigh[coeff][:])
			} else {
				// coefficients not used for this input shard (so null out)
				v := matrix[offset*32 : offset*32+32]
				for i := range v {
					v[i] = 0
				}
			}
			offset += dimIn
			if offset >= dimIn*dimOut82 {
				offset -= dimIn*dimOut82 - 1
			}
		}
	}
}

// Construct block of matrix coefficients for 4 output rows in parallel
func setupMatrix84(matrixRows [][]byte, inputOffset, outputOffset int, matrix *[matrixSize84]byte) {
	offset := 0
	for c := inputOffset; c < inputOffset+dimIn; c++ {
		for iRow := outputOffset; iRow < outputOffset+dimOut84; iRow++ {
			if c < len(matrixRows[iRow]) {
				coeff := matrixRows[iRow][c]
				copy(matrix[offset*32:], mulTableLow[coeff][:])
				copy(matrix[offset*32+16:], mulTableHigh[coeff][:])
			} else {
				// coefficients not used for this input shard (so null out)
				v := matrix[offset*32 : offset*32+32]
				for i := range v {
					v[i] = 0
				}
			}
			offset += dimIn
			if offset >= dimIn*dimOut84 {
				offset -= dimIn*dimOut84 - 1
			}
		}
	}
}

// Invoke AVX512 routine for single output row in parallel
func galMulAVX512Parallel81(in, out [][]byte, matrixRows [][]byte, inputOffset, outputOffset, start, stop int, matrix81 *[matrixSize81]byte) {
	done := stop - start
	if done <= 0 || len(in) == 0 || len(out) == 0 {
		return
	}

	inputEnd := inputOffset + dimIn
	if inputEnd > len(in) {
		inputEnd = len(in)
	}
	outputEnd := outputOffset + dimOut81
	if outputEnd > len(out) {
		outputEnd = len(out)
	}

	// We know the max size, alloc temp array.
	var inTmp [dimIn][]byte
	for i, v := range in[inputOffset:inputEnd] {
		inTmp[i] = v[start:stop]
	}
	var outTmp [dimOut81][]byte
	for i, v := range out[outputOffset:outputEnd] {
		outTmp[i] = v[start:stop]
	}

	addTo := inputOffset != 0 // Except for the first input column, add to previous results
	_galMulAVX512Parallel81(inTmp[:inputEnd-inputOffset], outTmp[:outputEnd-outputOffset], matrix81, addTo)

	done = start + ((done >> 6) << 6)
	if done < stop {
		galMulAVX512LastInput(inputOffset, inputEnd, outputOffset, outputEnd, matrixRows, done, stop, out, in)
	}
}

// Invoke AVX512 routine for 2 output rows in parallel
func galMulAVX512Parallel82(in, out [][]byte, matrixRows [][]byte, inputOffset, outputOffset, start, stop int, matrix82 *[matrixSize82]byte) {
	done := stop - start
	if done <= 0 || len(in) == 0 || len(out) == 0 {
		return
	}

	inputEnd := inputOffset + dimIn
	if inputEnd > len(in) {
		inputEnd = len(in)
	}
	outputEnd := outputOffset + dimOut82
	if outputEnd > len(out) {
		outputEnd = len(out)
	}

	// We know the max size, alloc temp array.
	var inTmp [dimIn][]byte
	for i, v := range in[inputOffset:inputEnd] {
		inTmp[i] = v[start:stop]
	}
	var outTmp [dimOut82][]byte
	for i, v := range out[outputOffset:outputEnd] {
		outTmp[i] = v[start:stop]
	}

	addTo := inputOffset != 0 // Except for the first input column, add to previous results
	_galMulAVX512Parallel82(inTmp[:inputEnd-inputOffset], outTmp[:outputEnd-outputOffset], matrix82, addTo)

	done = start + ((done >> 6) << 6)
	if done < stop {
		galMulAVX512LastInput(inputOffset, inputEnd, outputOffset, outputEnd, matrixRows, done, stop, out, in)
	}
}

// Invoke AVX512 routine for 4 output rows in parallel
func galMulAVX512Parallel84(in, out [][]byte, matrixRows [][]byte, inputOffset, outputOffset, start, stop int, matrix84 *[matrixSize84]byte) {
	done := stop - start
	if done <= 0 || len(in) == 0 || len(out) == 0 {
		return
	}

	inputEnd := inputOffset + dimIn
	if inputEnd > len(in) {
		inputEnd = len(in)
	}
	outputEnd := outputOffset + dimOut84
	if outputEnd > len(out) {
		outputEnd = len(out)
	}

	// We know the max size, alloc temp array.
	var inTmp [dimIn][]byte
	for i, v := range in[inputOffset:inputEnd] {
		inTmp[i] = v[start:stop]
	}
	var outTmp [dimOut84][]byte
	for i, v := range out[outputOffset:outputEnd] {
		outTmp[i] = v[start:stop]
	}

	addTo := inputOffset != 0 // Except for the first input column, add to previous results
	_galMulAVX512Parallel84(inTmp[:inputEnd-inputOffset], outTmp[:outputEnd-outputOffset], matrix84, addTo)

	done = start + ((done >> 6) << 6)
	if done < stop {
		galMulAVX512LastInput(inputOffset, inputEnd, outputOffset, outputEnd, matrixRows, done, stop, out, in)
	}
}

func galMulAVX512LastInput(inputOffset int, inputEnd int, outputOffset int, outputEnd int, matrixRows [][]byte, done int, stop int, out [][]byte, in [][]byte) {
	for c := inputOffset; c < inputEnd; c++ {
		for iRow := outputOffset; iRow < outputEnd; iRow++ {
			if c < len(matrixRows[iRow]) {
				mt := mulTable[matrixRows[iRow][c]][:256]
				for i := done; i < stop; i++ {
					if c == 0 { // only set value for first input column
						out[iRow][i] = mt[in[c][i]]
					} else { // and add for all others
						out[iRow][i] ^= mt[in[c][i]]
					}
				}
			}
		}
	}
}

// Perform the same as codeSomeShards, but taking advantage of
// AVX512 parallelism for up to 4x faster execution as compared to AVX2
func (r *reedSolomon) codeSomeShardsAvx512(matrixRows, inputs, outputs [][]byte, byteCount int) {
	// Process using no goroutines
	outputCount := len(outputs)
	start, end := 0, r.o.perRound
	if end > byteCount {
		end = byteCount
	}
	for start < byteCount {
		matrix84 := [matrixSize84]byte{}
		matrix82 := [matrixSize82]byte{}
		matrix81 := [matrixSize81]byte{}

		outputRow := 0
		// First process (multiple) batches of 4 output rows in parallel
		if outputRow+dimOut84 <= outputCount {
			for ; outputRow+dimOut84 <= outputCount; outputRow += dimOut84 {
				for inputRow := 0; inputRow < len(inputs); inputRow += dimIn {
					setupMatrix84(matrixRows, inputRow, outputRow, &matrix84)
					galMulAVX512Parallel84(inputs, outputs, matrixRows, inputRow, outputRow, start, end, &matrix84)
				}
			}
		}
		// Then process a (single) batch of 2 output rows in parallel
		if outputRow+dimOut82 <= outputCount {
			for inputRow := 0; inputRow < len(inputs); inputRow += dimIn {
				setupMatrix82(matrixRows, inputRow, outputRow, &matrix82)
				galMulAVX512Parallel82(inputs, outputs, matrixRows, inputRow, outputRow, start, end, &matrix82)
			}
			outputRow += dimOut82
		}
		// Lastly, we may have a single output row left (for uneven parity)
		if outputRow < outputCount {
			for inputRow := 0; inputRow < len(inputs); inputRow += dimIn {
				setupMatrix81(matrixRows, inputRow, outputRow, &matrix81)
				galMulAVX512Parallel81(inputs, outputs, matrixRows, inputRow, outputRow, start, end, &matrix81)
			}
		}

		start = end
		end += r.o.perRound
		if end > byteCount {
			end = byteCount
		}
	}
}

// Perform the same as codeSomeShards, but taking advantage of
// AVX512 parallelism for up to 4x faster execution as compared to AVX2
func (r *reedSolomon) codeSomeShardsAvx512P(matrixRows, inputs, outputs [][]byte, byteCount int) {
	outputCount := len(outputs)
	var wg sync.WaitGroup
	do := byteCount / r.o.maxGoroutines
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}
	// Make sizes divisible by 64
	do = (do + 63) & (^63)
	start := 0
	for start < byteCount {
		if start+do > byteCount {
			do = byteCount - start
		}
		wg.Add(1)
		go func(grStart, grStop int) {
			start, stop := grStart, grStart+r.o.perRound
			if stop > grStop {
				stop = grStop
			}
			// Loop for each round.
			matrix84 := [matrixSize84]byte{}
			matrix82 := [matrixSize82]byte{}
			matrix81 := [matrixSize81]byte{}
			for start < grStop {
				outputRow := 0
				// First process (multiple) batches of 4 output rows in parallel
				if outputRow+dimOut84 <= outputCount {
					// 1K matrix buffer
					for ; outputRow+dimOut84 <= outputCount; outputRow += dimOut84 {
						for inputRow := 0; inputRow < len(inputs); inputRow += dimIn {
							setupMatrix84(matrixRows, inputRow, outputRow, &matrix84)
							galMulAVX512Parallel84(inputs, outputs, matrixRows, inputRow, outputRow, start, stop, &matrix84)
						}
					}
				}
				// Then process a (single) batch of 2 output rows in parallel
				if outputRow+dimOut82 <= outputCount {
					// 512B matrix buffer
					for inputRow := 0; inputRow < len(inputs); inputRow += dimIn {
						setupMatrix82(matrixRows, inputRow, outputRow, &matrix82)
						galMulAVX512Parallel82(inputs, outputs, matrixRows, inputRow, outputRow, start, stop, &matrix82)
					}
					outputRow += dimOut82
				}
				// Lastly, we may have a single output row left (for uneven parity)
				if outputRow < outputCount {
					for inputRow := 0; inputRow < len(inputs); inputRow += dimIn {
						setupMatrix81(matrixRows, inputRow, outputRow, &matrix81)
						galMulAVX512Parallel81(inputs, outputs, matrixRows, inputRow, outputRow, start, stop, &matrix81)
					}
				}
				start = stop
				stop += r.o.perRound
				if stop > grStop {
					stop = grStop
				}
			}
			wg.Done()
		}(start, start+do)
		start += do
	}
	wg.Wait()
}
