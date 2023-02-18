/**
 * Reed-Solomon Coding over 8-bit values.
 *
 * Copyright 2015, Klaus Post
 * Copyright 2015, Backblaze, Inc.
 */

// Package reedsolomon enables Erasure Coding in Go
//
// For usage and examples, see https://github.com/klauspost/reedsolomon
//
package reedsolomon

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"sync"

	"github.com/klauspost/cpuid/v2"
)

// Encoder is an interface to encode Reed-Salomon parity sets for your data.
type Encoder interface {
	// Encode parity for a set of data shards.
	// Input is 'shards' containing data shards followed by parity shards.
	// The number of shards must match the number given to New().
	// Each shard is a byte array, and they must all be the same size.
	// The parity shards will always be overwritten and the data shards
	// will remain the same, so it is safe for you to read from the
	// data shards while this is running.
	Encode(shards [][]byte) error

	// EncodeIdx will add parity for a single data shard.
	// Parity shards should start out as 0. The caller must zero them.
	// Data shards must be delivered exactly once. There is no check for this.
	// The parity shards will always be updated and the data shards will remain the same.
	EncodeIdx(dataShard []byte, idx int, parity [][]byte) error

	// Verify returns true if the parity shards contain correct data.
	// The data is the same format as Encode. No data is modified, so
	// you are allowed to read from data while this is running.
	Verify(shards [][]byte) (bool, error)

	// Reconstruct will recreate the missing shards if possible.
	//
	// Given a list of shards, some of which contain data, fills in the
	// ones that don't have data.
	//
	// The length of the array must be equal to the total number of shards.
	// You indicate that a shard is missing by setting it to nil or zero-length.
	// If a shard is zero-length but has sufficient capacity, that memory will
	// be used, otherwise a new []byte will be allocated.
	//
	// If there are too few shards to reconstruct the missing
	// ones, ErrTooFewShards will be returned.
	//
	// The reconstructed shard set is complete, but integrity is not verified.
	// Use the Verify function to check if data set is ok.
	Reconstruct(shards [][]byte) error

	// ReconstructData will recreate any missing data shards, if possible.
	//
	// Given a list of shards, some of which contain data, fills in the
	// data shards that don't have data.
	//
	// The length of the array must be equal to Shards.
	// You indicate that a shard is missing by setting it to nil or zero-length.
	// If a shard is zero-length but has sufficient capacity, that memory will
	// be used, otherwise a new []byte will be allocated.
	//
	// If there are too few shards to reconstruct the missing
	// ones, ErrTooFewShards will be returned.
	//
	// As the reconstructed shard set may contain missing parity shards,
	// calling the Verify function is likely to fail.
	ReconstructData(shards [][]byte) error

	// ReconstructSome will recreate only requested data shards, if possible.
	//
	// Given a list of shards, some of which contain data, fills in the
	// data shards indicated by true values in the "required" parameter.
	// The length of "required" array must be equal to DataShards.
	//
	// The length of "shards" array must be equal to Shards.
	// You indicate that a shard is missing by setting it to nil or zero-length.
	// If a shard is zero-length but has sufficient capacity, that memory will
	// be used, otherwise a new []byte will be allocated.
	//
	// If there are too few shards to reconstruct the missing
	// ones, ErrTooFewShards will be returned.
	//
	// As the reconstructed shard set may contain missing parity shards,
	// calling the Verify function is likely to fail.
	ReconstructSome(shards [][]byte, required []bool) error

	// Update parity is use for change a few data shards and update it's parity.
	// Input 'newDatashards' containing data shards changed.
	// Input 'shards' containing old data shards (if data shard not changed, it can be nil) and old parity shards.
	// new parity shards will in shards[DataShards:]
	// Update is very useful if  DataShards much larger than ParityShards and changed data shards is few. It will
	// faster than Encode and not need read all data shards to encode.
	Update(shards [][]byte, newDatashards [][]byte) error

	// Split a data slice into the number of shards given to the encoder,
	// and create empty parity shards.
	//
	// The data will be split into equally sized shards.
	// If the data size isn't dividable by the number of shards,
	// the last shard will contain extra zeros.
	//
	// There must be at least 1 byte otherwise ErrShortData will be
	// returned.
	//
	// The data will not be copied, except for the last shard, so you
	// should not modify the data of the input slice afterwards.
	Split(data []byte) ([][]byte, error)

	// Join the shards and write the data segment to dst.
	//
	// Only the data shards are considered.
	// You must supply the exact output size you want.
	// If there are to few shards given, ErrTooFewShards will be returned.
	// If the total data size is less than outSize, ErrShortData will be returned.
	Join(dst io.Writer, shards [][]byte, outSize int) error
}

const (
	avx2CodeGenMinSize       = 64
	avx2CodeGenMinShards     = 3
	avx2CodeGenMaxGoroutines = 8

	intSize = 32 << (^uint(0) >> 63) // 32 or 64
	maxInt  = 1<<(intSize-1) - 1
)

// reedSolomon contains a matrix for a specific
// distribution of datashards and parity shards.
// Construct if using New()
type reedSolomon struct {
	DataShards   int // Number of data shards, should not be modified.
	ParityShards int // Number of parity shards, should not be modified.
	Shards       int // Total number of shards. Calculated, and should not be modified.
	m            matrix
	tree         *inversionTree
	parity       [][]byte
	o            options
	mPool        sync.Pool
}

// ErrInvShardNum will be returned by New, if you attempt to create
// an Encoder with less than one data shard or less than zero parity
// shards.
var ErrInvShardNum = errors.New("cannot create Encoder with less than one data shard or less than zero parity shards")

// ErrMaxShardNum will be returned by New, if you attempt to create an
// Encoder where data and parity shards are bigger than the order of
// GF(2^8).
var ErrMaxShardNum = errors.New("cannot create Encoder with more than 256 data+parity shards")

// buildMatrix creates the matrix to use for encoding, given the
// number of data shards and the number of total shards.
//
// The top square of the matrix is guaranteed to be an identity
// matrix, which means that the data shards are unchanged after
// encoding.
func buildMatrix(dataShards, totalShards int) (matrix, error) {
	// Start with a Vandermonde matrix.  This matrix would work,
	// in theory, but doesn't have the property that the data
	// shards are unchanged after encoding.
	vm, err := vandermonde(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	// Multiply by the inverse of the top square of the matrix.
	// This will make the top square be the identity matrix, but
	// preserve the property that any square subset of rows is
	// invertible.
	top, err := vm.SubMatrix(0, 0, dataShards, dataShards)
	if err != nil {
		return nil, err
	}

	topInv, err := top.Invert()
	if err != nil {
		return nil, err
	}

	return vm.Multiply(topInv)
}

// buildMatrixPAR1 creates the matrix to use for encoding according to
// the PARv1 spec, given the number of data shards and the number of
// total shards. Note that the method they use is buggy, and may lead
// to cases where recovery is impossible, even if there are enough
// parity shards.
//
// The top square of the matrix is guaranteed to be an identity
// matrix, which means that the data shards are unchanged after
// encoding.
func buildMatrixPAR1(dataShards, totalShards int) (matrix, error) {
	result, err := newMatrix(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	for r, row := range result {
		// The top portion of the matrix is the identity
		// matrix, and the bottom is a transposed Vandermonde
		// matrix starting at 1 instead of 0.
		if r < dataShards {
			result[r][r] = 1
		} else {
			for c := range row {
				result[r][c] = galExp(byte(c+1), r-dataShards)
			}
		}
	}
	return result, nil
}

func buildMatrixCauchy(dataShards, totalShards int) (matrix, error) {
	result, err := newMatrix(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	for r, row := range result {
		// The top portion of the matrix is the identity
		// matrix, and the bottom is a transposed Cauchy matrix.
		if r < dataShards {
			result[r][r] = 1
		} else {
			for c := range row {
				result[r][c] = invTable[(byte(r ^ c))]
			}
		}
	}
	return result, nil
}

// buildXorMatrix can be used to build a matrix with pure XOR
// operations if there is only one parity shard.
func buildXorMatrix(dataShards, totalShards int) (matrix, error) {
	if dataShards+1 != totalShards {
		return nil, errors.New("internal error")
	}
	result, err := newMatrix(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	for r, row := range result {
		// The top portion of the matrix is the identity
		// matrix.
		if r < dataShards {
			result[r][r] = 1
		} else {
			// Set all values to 1 (XOR)
			for c := range row {
				result[r][c] = 1
			}
		}
	}
	return result, nil
}

// New creates a new encoder and initializes it to
// the number of data shards and parity shards that
// you want to use. You can reuse this encoder.
// Note that the maximum number of total shards is 256.
// If no options are supplied, default options are used.
func New(dataShards, parityShards int, opts ...Option) (Encoder, error) {
	r := reedSolomon{
		DataShards:   dataShards,
		ParityShards: parityShards,
		Shards:       dataShards + parityShards,
		o:            defaultOptions,
	}

	for _, opt := range opts {
		opt(&r.o)
	}
	if dataShards <= 0 || parityShards < 0 {
		return nil, ErrInvShardNum
	}

	if dataShards+parityShards > 256 {
		return nil, ErrMaxShardNum
	}

	if parityShards == 0 {
		return &r, nil
	}

	var err error
	switch {
	case r.o.customMatrix != nil:
		if len(r.o.customMatrix) < parityShards {
			return nil, errors.New("coding matrix must contain at least parityShards rows")
		}
		r.m = make([][]byte, r.Shards)
		for i := 0; i < dataShards; i++ {
			r.m[i] = make([]byte, dataShards)
			r.m[i][i] = 1
		}
		for k, row := range r.o.customMatrix {
			if len(row) < dataShards {
				return nil, errors.New("coding matrix must contain at least dataShards columns")
			}
			r.m[dataShards+k] = make([]byte, dataShards)
			copy(r.m[dataShards+k], row)
		}
	case r.o.fastOneParity && parityShards == 1:
		r.m, err = buildXorMatrix(dataShards, r.Shards)
	case r.o.useCauchy:
		r.m, err = buildMatrixCauchy(dataShards, r.Shards)
	case r.o.usePAR1Matrix:
		r.m, err = buildMatrixPAR1(dataShards, r.Shards)
	default:
		r.m, err = buildMatrix(dataShards, r.Shards)
	}
	if err != nil {
		return nil, err
	}

	// Calculate what we want per round
	r.o.perRound = cpuid.CPU.Cache.L2

	divide := parityShards + 1
	if avx2CodeGen && r.o.useAVX2 && (dataShards > maxAvx2Inputs || parityShards > maxAvx2Outputs) {
		// Base on L1 cache if we have many inputs.
		r.o.perRound = cpuid.CPU.Cache.L1D
		divide = 0
		if dataShards > maxAvx2Inputs {
			divide += maxAvx2Inputs
		} else {
			divide += dataShards
		}
		if parityShards > maxAvx2Inputs {
			divide += maxAvx2Outputs
		} else {
			divide += parityShards
		}
	}

	if r.o.perRound <= 0 {
		// Set to 128K if undetectable.
		r.o.perRound = 128 << 10
	}

	if cpuid.CPU.ThreadsPerCore > 1 && r.o.maxGoroutines > cpuid.CPU.PhysicalCores {
		// If multiple threads per core, make sure they don't contend for cache.
		r.o.perRound /= cpuid.CPU.ThreadsPerCore
	}

	// 1 input + parity must fit in cache, and we add one more to be safer.
	r.o.perRound = r.o.perRound / divide
	// Align to 64 bytes.
	r.o.perRound = ((r.o.perRound + 63) / 64) * 64

	if r.o.minSplitSize <= 0 {
		// Set minsplit as high as we can, but still have parity in L1.
		cacheSize := cpuid.CPU.Cache.L1D
		if cacheSize <= 0 {
			cacheSize = 32 << 10
		}

		r.o.minSplitSize = cacheSize / (parityShards + 1)
		// Min 1K
		if r.o.minSplitSize < 1024 {
			r.o.minSplitSize = 1024
		}
	}

	if r.o.shardSize > 0 {
		p := runtime.GOMAXPROCS(0)
		if p == 1 || r.o.shardSize <= r.o.minSplitSize*2 {
			// Not worth it.
			r.o.maxGoroutines = 1
		} else {
			g := r.o.shardSize / r.o.perRound

			// Overprovision by a factor of 2.
			if g < p*2 && r.o.perRound > r.o.minSplitSize*2 {
				g = p * 2
				r.o.perRound /= 2
			}

			// Have g be multiple of p
			g += p - 1
			g -= g % p

			r.o.maxGoroutines = g
		}
	}

	// Generated AVX2 does not need data to stay in L1 cache between runs.
	// We will be purely limited by RAM speed.
	if r.canAVX2C(avx2CodeGenMinSize, maxAvx2Inputs, maxAvx2Outputs) && r.o.maxGoroutines > avx2CodeGenMaxGoroutines {
		r.o.maxGoroutines = avx2CodeGenMaxGoroutines
	}

	// Inverted matrices are cached in a tree keyed by the indices
	// of the invalid rows of the data to reconstruct.
	// The inversion root node will have the identity matrix as
	// its inversion matrix because it implies there are no errors
	// with the original data.
	if r.o.inversionCache {
		r.tree = newInversionTree(dataShards, parityShards)
	}

	r.parity = make([][]byte, parityShards)
	for i := range r.parity {
		r.parity[i] = r.m[dataShards+i]
	}

	if avx2CodeGen && r.o.useAVX2 {
		sz := r.DataShards * r.ParityShards * 2 * 32
		r.mPool.New = func() interface{} {
			return make([]byte, sz)
		}
	}
	return &r, err
}

// ErrTooFewShards is returned if too few shards where given to
// Encode/Verify/Reconstruct/Update. It will also be returned from Reconstruct
// if there were too few shards to reconstruct the missing data.
var ErrTooFewShards = errors.New("too few shards given")

// Encode parity for a set of data shards.
// An array 'shards' containing data shards followed by parity shards.
// The number of shards must match the number given to New.
// Each shard is a byte array, and they must all be the same size.
// The parity shards will always be overwritten and the data shards
// will remain the same.
func (r *reedSolomon) Encode(shards [][]byte) error {
	if len(shards) != r.Shards {
		return ErrTooFewShards
	}

	err := checkShards(shards, false)
	if err != nil {
		return err
	}

	// Get the slice of output buffers.
	output := shards[r.DataShards:]

	// Do the coding.
	r.codeSomeShards(r.parity, shards[0:r.DataShards], output[:r.ParityShards], len(shards[0]))
	return nil
}

// EncodeIdx will add parity for a single data shard.
// Parity shards should start out zeroed. The caller must zero them before first call.
// Data shards should only be delivered once. There is no check for this.
// The parity shards will always be updated and the data shards will remain the unchanged.
func (r *reedSolomon) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	if len(parity) != r.ParityShards {
		return ErrTooFewShards
	}
	if len(parity) == 0 {
		return nil
	}
	if idx < 0 || idx >= r.DataShards {
		return ErrInvShardNum
	}
	err := checkShards(parity, false)
	if err != nil {
		return err
	}
	if len(parity[0]) != len(dataShard) {
		return ErrShardSize
	}

	// Process using no goroutines for now.
	start, end := 0, r.o.perRound
	if end > len(dataShard) {
		end = len(dataShard)
	}

	for start < len(dataShard) {
		in := dataShard[start:end]
		for iRow := 0; iRow < r.ParityShards; iRow++ {
			galMulSliceXor(r.parity[iRow][idx], in, parity[iRow][start:end], &r.o)
		}
		start = end
		end += r.o.perRound
		if end > len(dataShard) {
			end = len(dataShard)
		}
	}
	return nil
}

// ErrInvalidInput is returned if invalid input parameter of Update.
var ErrInvalidInput = errors.New("invalid input")

func (r *reedSolomon) Update(shards [][]byte, newDatashards [][]byte) error {
	if len(shards) != r.Shards {
		return ErrTooFewShards
	}

	if len(newDatashards) != r.DataShards {
		return ErrTooFewShards
	}

	err := checkShards(shards, true)
	if err != nil {
		return err
	}

	err = checkShards(newDatashards, true)
	if err != nil {
		return err
	}

	for i := range newDatashards {
		if newDatashards[i] != nil && shards[i] == nil {
			return ErrInvalidInput
		}
	}
	for _, p := range shards[r.DataShards:] {
		if p == nil {
			return ErrInvalidInput
		}
	}

	shardSize := shardSize(shards)

	// Get the slice of output buffers.
	output := shards[r.DataShards:]

	// Do the coding.
	r.updateParityShards(r.parity, shards[0:r.DataShards], newDatashards[0:r.DataShards], output, r.ParityShards, shardSize)
	return nil
}

func (r *reedSolomon) updateParityShards(matrixRows, oldinputs, newinputs, outputs [][]byte, outputCount, byteCount int) {
	if len(outputs) == 0 {
		return
	}

	if r.o.maxGoroutines > 1 && byteCount > r.o.minSplitSize {
		r.updateParityShardsP(matrixRows, oldinputs, newinputs, outputs, outputCount, byteCount)
		return
	}

	for c := 0; c < r.DataShards; c++ {
		in := newinputs[c]
		if in == nil {
			continue
		}
		oldin := oldinputs[c]
		// oldinputs data will be changed
		sliceXor(in, oldin, &r.o)
		for iRow := 0; iRow < outputCount; iRow++ {
			galMulSliceXor(matrixRows[iRow][c], oldin, outputs[iRow], &r.o)
		}
	}
}

func (r *reedSolomon) updateParityShardsP(matrixRows, oldinputs, newinputs, outputs [][]byte, outputCount, byteCount int) {
	var wg sync.WaitGroup
	do := byteCount / r.o.maxGoroutines
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}
	start := 0
	for start < byteCount {
		if start+do > byteCount {
			do = byteCount - start
		}
		wg.Add(1)
		go func(start, stop int) {
			for c := 0; c < r.DataShards; c++ {
				in := newinputs[c]
				if in == nil {
					continue
				}
				oldin := oldinputs[c]
				// oldinputs data will be change
				sliceXor(in[start:stop], oldin[start:stop], &r.o)
				for iRow := 0; iRow < outputCount; iRow++ {
					galMulSliceXor(matrixRows[iRow][c], oldin[start:stop], outputs[iRow][start:stop], &r.o)
				}
			}
			wg.Done()
		}(start, start+do)
		start += do
	}
	wg.Wait()
}

// Verify returns true if the parity shards contain the right data.
// The data is the same format as Encode. No data is modified.
func (r *reedSolomon) Verify(shards [][]byte) (bool, error) {
	if len(shards) != r.Shards {
		return false, ErrTooFewShards
	}
	err := checkShards(shards, false)
	if err != nil {
		return false, err
	}

	// Slice of buffers being checked.
	toCheck := shards[r.DataShards:]

	// Do the checking.
	return r.checkSomeShards(r.parity, shards[:r.DataShards], toCheck[:r.ParityShards], len(shards[0])), nil
}

func (r *reedSolomon) canAVX2C(byteCount int, inputs, outputs int) bool {
	return avx2CodeGen && r.o.useAVX2 &&
		byteCount >= avx2CodeGenMinSize && inputs+outputs >= avx2CodeGenMinShards &&
		inputs <= maxAvx2Inputs && outputs <= maxAvx2Outputs
}

// Multiplies a subset of rows from a coding matrix by a full set of
// input shards to produce some output shards.
// 'matrixRows' is The rows from the matrix to use.
// 'inputs' An array of byte arrays, each of which is one input shard.
// The number of inputs used is determined by the length of each matrix row.
// outputs Byte arrays where the computed shards are stored.
// The number of outputs computed, and the
// number of matrix rows used, is determined by
// outputCount, which is the number of outputs to compute.
func (r *reedSolomon) codeSomeShards(matrixRows, inputs, outputs [][]byte, byteCount int) {
	if len(outputs) == 0 {
		return
	}
	switch {
	case r.o.useAVX512 && r.o.maxGoroutines > 1 && byteCount > r.o.minSplitSize && len(inputs) >= 4 && len(outputs) >= 2:
		r.codeSomeShardsAvx512P(matrixRows, inputs, outputs, byteCount)
		return
	case r.o.useAVX512 && len(inputs) >= 4 && len(outputs) >= 2:
		r.codeSomeShardsAvx512(matrixRows, inputs, outputs, byteCount)
		return
	case byteCount > r.o.minSplitSize:
		r.codeSomeShardsP(matrixRows, inputs, outputs, byteCount)
		return
	}

	// Process using no goroutines
	start, end := 0, r.o.perRound
	if end > len(inputs[0]) {
		end = len(inputs[0])
	}
	if r.canAVX2C(byteCount, len(inputs), len(outputs)) {
		m := genAvx2Matrix(matrixRows, len(inputs), 0, len(outputs), r.mPool.Get().([]byte))
		start += galMulSlicesAvx2(m, inputs, outputs, 0, byteCount)
		r.mPool.Put(m)
		end = len(inputs[0])
	} else if len(inputs)+len(outputs) > avx2CodeGenMinShards && r.canAVX2C(byteCount, maxAvx2Inputs, maxAvx2Outputs) {
		end = len(inputs[0])
		inIdx := 0
		m := r.mPool.Get().([]byte)
		defer r.mPool.Put(m)
		ins := inputs
		for len(ins) > 0 {
			inPer := ins
			if len(inPer) > maxAvx2Inputs {
				inPer = inPer[:maxAvx2Inputs]
			}
			outs := outputs
			outIdx := 0
			for len(outs) > 0 {
				outPer := outs
				if len(outPer) > maxAvx2Outputs {
					outPer = outPer[:maxAvx2Outputs]
				}
				m = genAvx2Matrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), m)
				if inIdx == 0 {
					galMulSlicesAvx2(m, inPer, outPer, 0, byteCount)
				} else {
					galMulSlicesAvx2Xor(m, inPer, outPer, 0, byteCount)
				}
				start = byteCount & avxSizeMask
				outIdx += len(outPer)
				outs = outs[len(outPer):]
			}
			inIdx += len(inPer)
			ins = ins[len(inPer):]
		}
		if start >= end {
			return
		}
	}
	for start < len(inputs[0]) {
		for c := 0; c < len(inputs); c++ {
			in := inputs[c][start:end]
			for iRow := 0; iRow < len(outputs); iRow++ {
				if c == 0 {
					galMulSlice(matrixRows[iRow][c], in, outputs[iRow][start:end], &r.o)
				} else {
					galMulSliceXor(matrixRows[iRow][c], in, outputs[iRow][start:end], &r.o)
				}
			}
		}
		start = end
		end += r.o.perRound
		if end > len(inputs[0]) {
			end = len(inputs[0])
		}
	}
}

// Perform the same as codeSomeShards, but split the workload into
// several goroutines.
func (r *reedSolomon) codeSomeShardsP(matrixRows, inputs, outputs [][]byte, byteCount int) {
	var wg sync.WaitGroup
	gor := r.o.maxGoroutines

	var avx2Matrix []byte
	useAvx2 := r.canAVX2C(byteCount, len(inputs), len(outputs))
	if useAvx2 {
		avx2Matrix = genAvx2Matrix(matrixRows, len(inputs), 0, len(outputs), r.mPool.Get().([]byte))
		defer r.mPool.Put(avx2Matrix)
	} else if byteCount < 10<<20 && len(inputs)+len(outputs) > avx2CodeGenMinShards &&
		r.canAVX2C(byteCount/4, maxAvx2Inputs, maxAvx2Outputs) {
		// It appears there is a switchover point at around 10MB where
		// Regular processing is faster...
		r.codeSomeShardsAVXP(matrixRows, inputs, outputs, byteCount)
		return
	}

	do := byteCount / gor
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}

	exec := func(start, stop int) {
		if useAvx2 && stop-start >= 64 {
			start += galMulSlicesAvx2(avx2Matrix, inputs, outputs, start, stop)
		}

		lstart, lstop := start, start+r.o.perRound
		if lstop > stop {
			lstop = stop
		}
		for lstart < stop {
			for c := 0; c < len(inputs); c++ {
				in := inputs[c][lstart:lstop]
				for iRow := 0; iRow < len(outputs); iRow++ {
					if c == 0 {
						galMulSlice(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					} else {
						galMulSliceXor(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					}
				}
			}
			lstart = lstop
			lstop += r.o.perRound
			if lstop > stop {
				lstop = stop
			}
		}
		wg.Done()
	}
	if gor <= 1 {
		wg.Add(1)
		exec(0, byteCount)
		return
	}

	// Make sizes divisible by 64
	do = (do + 63) & (^63)
	start := 0
	for start < byteCount {
		if start+do > byteCount {
			do = byteCount - start
		}

		wg.Add(1)
		go exec(start, start+do)
		start += do
	}
	wg.Wait()
}

// Perform the same as codeSomeShards, but split the workload into
// several goroutines.
func (r *reedSolomon) codeSomeShardsAVXP(matrixRows, inputs, outputs [][]byte, byteCount int) {
	var wg sync.WaitGroup
	gor := r.o.maxGoroutines

	type state struct {
		input  [][]byte
		output [][]byte
		m      []byte
		first  bool
	}
	// Make a plan...
	plan := make([]state, 0, ((len(inputs)+maxAvx2Inputs-1)/maxAvx2Inputs)*((len(outputs)+maxAvx2Outputs-1)/maxAvx2Outputs))

	tmp := r.mPool.Get().([]byte)
	defer func(b []byte) {
		r.mPool.Put(b)
	}(tmp)

	// Flips between input first to output first.
	// We put the smallest data load in the inner loop.
	if len(inputs) > len(outputs) {
		inIdx := 0
		ins := inputs
		for len(ins) > 0 {
			inPer := ins
			if len(inPer) > maxAvx2Inputs {
				inPer = inPer[:maxAvx2Inputs]
			}
			outs := outputs
			outIdx := 0
			for len(outs) > 0 {
				outPer := outs
				if len(outPer) > maxAvx2Outputs {
					outPer = outPer[:maxAvx2Outputs]
				}
				// Generate local matrix
				m := genAvx2Matrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), tmp)
				tmp = tmp[len(m):]
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0,
				})
				outIdx += len(outPer)
				outs = outs[len(outPer):]
			}
			inIdx += len(inPer)
			ins = ins[len(inPer):]
		}
	} else {
		outs := outputs
		outIdx := 0
		for len(outs) > 0 {
			outPer := outs
			if len(outPer) > maxAvx2Outputs {
				outPer = outPer[:maxAvx2Outputs]
			}

			inIdx := 0
			ins := inputs
			for len(ins) > 0 {
				inPer := ins
				if len(inPer) > maxAvx2Inputs {
					inPer = inPer[:maxAvx2Inputs]
				}
				// Generate local matrix
				m := genAvx2Matrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), tmp)
				tmp = tmp[len(m):]
				//fmt.Println("bytes:", len(inPer)*r.o.perRound, "out:", len(outPer)*r.o.perRound)
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0,
				})
				inIdx += len(inPer)
				ins = ins[len(inPer):]
			}
			outIdx += len(outPer)
			outs = outs[len(outPer):]
		}
	}

	do := byteCount / gor
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}

	exec := func(start, stop int) {
		lstart, lstop := start, start+r.o.perRound
		if lstop > stop {
			lstop = stop
		}
		for lstart < stop {
			if lstop-lstart >= minAvx2Size {
				// Execute plan...
				for _, p := range plan {
					if p.first {
						galMulSlicesAvx2(p.m, p.input, p.output, lstart, lstop)
					} else {
						galMulSlicesAvx2Xor(p.m, p.input, p.output, lstart, lstop)
					}
				}
				lstart += (lstop - lstart) & avxSizeMask
				if lstart == lstop {
					lstop += r.o.perRound
					if lstop > stop {
						lstop = stop
					}
					continue
				}
			}

			for c := range inputs {
				in := inputs[c][lstart:lstop]
				for iRow := 0; iRow < len(outputs); iRow++ {
					if c == 0 {
						galMulSlice(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					} else {
						galMulSliceXor(matrixRows[iRow][c], in, outputs[iRow][lstart:lstop], &r.o)
					}
				}
			}
			lstart = lstop
			lstop += r.o.perRound
			if lstop > stop {
				lstop = stop
			}
		}
		wg.Done()
	}
	if gor == 1 {
		wg.Add(1)
		exec(0, byteCount)
		return
	}

	// Make sizes divisible by 64
	do = (do + 63) & (^63)
	start := 0
	for start < byteCount {
		if start+do > byteCount {
			do = byteCount - start
		}

		wg.Add(1)
		go exec(start, start+do)
		start += do
	}
	wg.Wait()
}

// checkSomeShards is mostly the same as codeSomeShards,
// except this will check values and return
// as soon as a difference is found.
func (r *reedSolomon) checkSomeShards(matrixRows, inputs, toCheck [][]byte, byteCount int) bool {
	if len(toCheck) == 0 {
		return true
	}

	outputs := make([][]byte, len(toCheck))
	for i := range outputs {
		outputs[i] = make([]byte, byteCount)
	}
	r.codeSomeShards(matrixRows, inputs, outputs, byteCount)

	for i, calc := range outputs {
		if !bytes.Equal(calc, toCheck[i]) {
			return false
		}
	}
	return true
}

// ErrShardNoData will be returned if there are no shards,
// or if the length of all shards is zero.
var ErrShardNoData = errors.New("no shard data")

// ErrShardSize is returned if shard length isn't the same for all
// shards.
var ErrShardSize = errors.New("shard sizes do not match")

// checkShards will check if shards are the same size
// or 0, if allowed. An error is returned if this fails.
// An error is also returned if all shards are size 0.
func checkShards(shards [][]byte, nilok bool) error {
	size := shardSize(shards)
	if size == 0 {
		return ErrShardNoData
	}
	for _, shard := range shards {
		if len(shard) != size {
			if len(shard) != 0 || !nilok {
				return ErrShardSize
			}
		}
	}
	return nil
}

// shardSize return the size of a single shard.
// The first non-zero size is returned,
// or 0 if all shards are size 0.
func shardSize(shards [][]byte) int {
	for _, shard := range shards {
		if len(shard) != 0 {
			return len(shard)
		}
	}
	return 0
}

// Reconstruct will recreate the missing shards, if possible.
//
// Given a list of shards, some of which contain data, fills in the
// ones that don't have data.
//
// The length of the array must be equal to Shards.
// You indicate that a shard is missing by setting it to nil or zero-length.
// If a shard is zero-length but has sufficient capacity, that memory will
// be used, otherwise a new []byte will be allocated.
//
// If there are too few shards to reconstruct the missing
// ones, ErrTooFewShards will be returned.
//
// The reconstructed shard set is complete, but integrity is not verified.
// Use the Verify function to check if data set is ok.
func (r *reedSolomon) Reconstruct(shards [][]byte) error {
	return r.reconstruct(shards, false, nil)
}

// ReconstructData will recreate any missing data shards, if possible.
//
// Given a list of shards, some of which contain data, fills in the
// data shards that don't have data.
//
// The length of the array must be equal to Shards.
// You indicate that a shard is missing by setting it to nil or zero-length.
// If a shard is zero-length but has sufficient capacity, that memory will
// be used, otherwise a new []byte will be allocated.
//
// If there are too few shards to reconstruct the missing
// ones, ErrTooFewShards will be returned.
//
// As the reconstructed shard set may contain missing parity shards,
// calling the Verify function is likely to fail.
func (r *reedSolomon) ReconstructData(shards [][]byte) error {
	return r.reconstruct(shards, true, nil)
}

// ReconstructSome will recreate only requested data shards, if possible.
//
// Given a list of shards, some of which contain data, fills in the
// data shards indicated by true values in the "required" parameter.
// The length of "required" array must be equal to DataShards.
//
// The length of "shards" array must be equal to Shards.
// You indicate that a shard is missing by setting it to nil or zero-length.
// If a shard is zero-length but has sufficient capacity, that memory will
// be used, otherwise a new []byte will be allocated.
//
// If there are too few shards to reconstruct the missing
// ones, ErrTooFewShards will be returned.
//
// As the reconstructed shard set may contain missing parity shards,
// calling the Verify function is likely to fail.
func (r *reedSolomon) ReconstructSome(shards [][]byte, required []bool) error {
	return r.reconstruct(shards, true, required)
}

// reconstruct will recreate the missing data shards, and unless
// dataOnly is true, also the missing parity shards
//
// The length of "shards" array must be equal to Shards.
// You indicate that a shard is missing by setting it to nil.
//
// If there are too few shards to reconstruct the missing
// ones, ErrTooFewShards will be returned.
func (r *reedSolomon) reconstruct(shards [][]byte, dataOnly bool, required []bool) error {
	if len(shards) != r.Shards || required != nil && len(required) < r.DataShards {
		return ErrTooFewShards
	}
	// Check arguments.
	err := checkShards(shards, true)
	if err != nil {
		return err
	}

	shardSize := shardSize(shards)

	// Quick check: are all of the shards present?  If so, there's
	// nothing to do.
	numberPresent := 0
	dataPresent := 0
	missingRequired := 0
	for i := 0; i < r.Shards; i++ {
		if len(shards[i]) != 0 {
			numberPresent++
			if i < r.DataShards {
				dataPresent++
			}
		} else if required != nil && required[i] {
			missingRequired++
		}
	}
	if numberPresent == r.Shards || dataOnly && dataPresent == r.DataShards ||
		required != nil && missingRequired == 0 {
		// Cool.  All of the shards data data.  We don't
		// need to do anything.
		return nil
	}

	// More complete sanity check
	if numberPresent < r.DataShards {
		return ErrTooFewShards
	}

	// Pull out an array holding just the shards that
	// correspond to the rows of the submatrix.  These shards
	// will be the input to the decoding process that re-creates
	// the missing data shards.
	//
	// Also, create an array of indices of the valid rows we do have
	// and the invalid rows we don't have up until we have enough valid rows.
	subShards := make([][]byte, r.DataShards)
	validIndices := make([]int, r.DataShards)
	invalidIndices := make([]int, 0)
	subMatrixRow := 0
	for matrixRow := 0; matrixRow < r.Shards && subMatrixRow < r.DataShards; matrixRow++ {
		if len(shards[matrixRow]) != 0 {
			subShards[subMatrixRow] = shards[matrixRow]
			validIndices[subMatrixRow] = matrixRow
			subMatrixRow++
		} else {
			invalidIndices = append(invalidIndices, matrixRow)
		}
	}

	// Attempt to get the cached inverted matrix out of the tree
	// based on the indices of the invalid rows.
	dataDecodeMatrix := r.tree.GetInvertedMatrix(invalidIndices)

	// If the inverted matrix isn't cached in the tree yet we must
	// construct it ourselves and insert it into the tree for the
	// future.  In this way the inversion tree is lazily loaded.
	if dataDecodeMatrix == nil {
		// Pull out the rows of the matrix that correspond to the
		// shards that we have and build a square matrix.  This
		// matrix could be used to generate the shards that we have
		// from the original data.
		subMatrix, _ := newMatrix(r.DataShards, r.DataShards)
		for subMatrixRow, validIndex := range validIndices {
			for c := 0; c < r.DataShards; c++ {
				subMatrix[subMatrixRow][c] = r.m[validIndex][c]
			}
		}
		// Invert the matrix, so we can go from the encoded shards
		// back to the original data.  Then pull out the row that
		// generates the shard that we want to decode.  Note that
		// since this matrix maps back to the original data, it can
		// be used to create a data shard, but not a parity shard.
		dataDecodeMatrix, err = subMatrix.Invert()
		if err != nil {
			return err
		}

		// Cache the inverted matrix in the tree for future use keyed on the
		// indices of the invalid rows.
		err = r.tree.InsertInvertedMatrix(invalidIndices, dataDecodeMatrix, r.Shards)
		if err != nil {
			return err
		}
	}

	// Re-create any data shards that were missing.
	//
	// The input to the coding is all of the shards we actually
	// have, and the output is the missing data shards.  The computation
	// is done using the special decode matrix we just built.
	outputs := make([][]byte, r.ParityShards)
	matrixRows := make([][]byte, r.ParityShards)
	outputCount := 0

	for iShard := 0; iShard < r.DataShards; iShard++ {
		if len(shards[iShard]) == 0 && (required == nil || required[iShard]) {
			if cap(shards[iShard]) >= shardSize {
				shards[iShard] = shards[iShard][0:shardSize]
			} else {
				shards[iShard] = make([]byte, shardSize)
			}
			outputs[outputCount] = shards[iShard]
			matrixRows[outputCount] = dataDecodeMatrix[iShard]
			outputCount++
		}
	}
	r.codeSomeShards(matrixRows, subShards, outputs[:outputCount], shardSize)

	if dataOnly {
		// Exit out early if we are only interested in the data shards
		return nil
	}

	// Now that we have all of the data shards intact, we can
	// compute any of the parity that is missing.
	//
	// The input to the coding is ALL of the data shards, including
	// any that we just calculated.  The output is whichever of the
	// data shards were missing.
	outputCount = 0
	for iShard := r.DataShards; iShard < r.Shards; iShard++ {
		if len(shards[iShard]) == 0 && (required == nil || required[iShard]) {
			if cap(shards[iShard]) >= shardSize {
				shards[iShard] = shards[iShard][0:shardSize]
			} else {
				shards[iShard] = make([]byte, shardSize)
			}
			outputs[outputCount] = shards[iShard]
			matrixRows[outputCount] = r.parity[iShard-r.DataShards]
			outputCount++
		}
	}
	r.codeSomeShards(matrixRows, shards[:r.DataShards], outputs[:outputCount], shardSize)
	return nil
}

// ErrShortData will be returned by Split(), if there isn't enough data
// to fill the number of shards.
var ErrShortData = errors.New("not enough data to fill the number of requested shards")

// Split a data slice into the number of shards given to the encoder,
// and create empty parity shards if necessary.
//
// The data will be split into equally sized shards.
// If the data size isn't divisible by the number of shards,
// the last shard will contain extra zeros.
//
// There must be at least 1 byte otherwise ErrShortData will be
// returned.
//
// The data will not be copied, except for the last shard, so you
// should not modify the data of the input slice afterwards.
func (r *reedSolomon) Split(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, ErrShortData
	}
	dataLen := len(data)
	// Calculate number of bytes per data shard.
	perShard := (len(data) + r.DataShards - 1) / r.DataShards

	if cap(data) > len(data) {
		data = data[:cap(data)]
	}

	// Only allocate memory if necessary
	var padding []byte
	if len(data) < (r.Shards * perShard) {
		// calculate maximum number of full shards in `data` slice
		fullShards := len(data) / perShard
		padding = make([]byte, r.Shards*perShard-perShard*fullShards)
		copy(padding, data[perShard*fullShards:])
		data = data[0 : perShard*fullShards]
	} else {
		for i := dataLen; i < dataLen+r.DataShards; i++ {
			data[i] = 0
		}
	}

	// Split into equal-length shards.
	dst := make([][]byte, r.Shards)
	i := 0
	for ; i < len(dst) && len(data) >= perShard; i++ {
		dst[i] = data[:perShard:perShard]
		data = data[perShard:]
	}

	for j := 0; i+j < len(dst); j++ {
		dst[i+j] = padding[:perShard:perShard]
		padding = padding[perShard:]
	}

	return dst, nil
}

// ErrReconstructRequired is returned if too few data shards are intact and a
// reconstruction is required before you can successfully join the shards.
var ErrReconstructRequired = errors.New("reconstruction required as one or more required data shards are nil")

// Join the shards and write the data segment to dst.
//
// Only the data shards are considered.
// You must supply the exact output size you want.
//
// If there are to few shards given, ErrTooFewShards will be returned.
// If the total data size is less than outSize, ErrShortData will be returned.
// If one or more required data shards are nil, ErrReconstructRequired will be returned.
func (r *reedSolomon) Join(dst io.Writer, shards [][]byte, outSize int) error {
	// Do we have enough shards?
	if len(shards) < r.DataShards {
		return ErrTooFewShards
	}
	shards = shards[:r.DataShards]

	// Do we have enough data?
	size := 0
	for _, shard := range shards {
		if shard == nil {
			return ErrReconstructRequired
		}
		size += len(shard)

		// Do we have enough data already?
		if size >= outSize {
			break
		}
	}
	if size < outSize {
		return ErrShortData
	}

	// Copy data to dst
	write := outSize
	for _, shard := range shards {
		if write < len(shard) {
			_, err := dst.Write(shard[:write])
			return err
		}
		n, err := dst.Write(shard)
		if err != nil {
			return err
		}
		write -= n
	}
	return nil
}
