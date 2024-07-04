/**
 * Reed-Solomon Coding over 8-bit values.
 *
 * Copyright 2015, Klaus Post
 * Copyright 2015, Backblaze, Inc.
 */

// Package reedsolomon enables Erasure Coding in Go
//
// For usage and examples, see https://github.com/klauspost/reedsolomon
package reedsolomon

import (
	"bytes"
	"errors"
	"fmt"
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

	// ReconstructSome will recreate only requested shards, if possible.
	//
	// Given a list of shards, some of which contain data, fills in the
	// shards indicated by true values in the "required" parameter.
	// The length of the "required" array must be equal to either Shards or DataShards.
	// If the length is equal to DataShards, the reconstruction of parity shards will be ignored.
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
	// and create empty parity shards if necessary.
	//
	// The data will be split into equally sized shards.
	// If the data size isn't divisible by the number of shards,
	// the last shard will contain extra zeros.
	//
	// If there is extra capacity on the provided data slice
	// it will be used instead of allocating parity shards.
	// It will be zeroed out.
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

// Extensions is an optional interface.
// All returned instances will support this interface.
type Extensions interface {
	// ShardSizeMultiple will return the size the shard sizes must be a multiple of.
	ShardSizeMultiple() int

	// DataShards will return the number of data shards.
	DataShards() int

	// ParityShards will return the number of parity shards.
	ParityShards() int

	// TotalShards will return the total number of shards.
	TotalShards() int

	// AllocAligned will allocate TotalShards number of slices,
	// aligned to reasonable memory sizes.
	// Provide the size of each shard.
	AllocAligned(each int) [][]byte
}

const (
	avx2CodeGenMinSize       = 64
	avx2CodeGenMinShards     = 3
	avx2CodeGenMaxGoroutines = 8
	gfniCodeGenMaxGoroutines = 4

	intSize = 32 << (^uint(0) >> 63) // 32 or 64
	maxInt  = 1<<(intSize-1) - 1
)

// reedSolomon contains a matrix for a specific
// distribution of datashards and parity shards.
// Construct if using New()
type reedSolomon struct {
	dataShards   int // Number of data shards, should not be modified.
	parityShards int // Number of parity shards, should not be modified.
	totalShards  int // Total number of shards. Calculated, and should not be modified.
	m            matrix
	tree         *inversionTree
	parity       [][]byte
	o            options
	mPoolSz      int
	mPool        sync.Pool // Pool for temp matrices, etc
}

var _ = Extensions(&reedSolomon{})

func (r *reedSolomon) ShardSizeMultiple() int {
	return 1
}

func (r *reedSolomon) DataShards() int {
	return r.dataShards
}

func (r *reedSolomon) ParityShards() int {
	return r.parityShards
}

func (r *reedSolomon) TotalShards() int {
	return r.totalShards
}

func (r *reedSolomon) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each)
}

// ErrInvShardNum will be returned by New, if you attempt to create
// an Encoder with less than one data shard or less than zero parity
// shards.
var ErrInvShardNum = errors.New("cannot create Encoder with less than one data shard or less than zero parity shards")

// ErrMaxShardNum will be returned by New, if you attempt to create an
// Encoder where data and parity shards are bigger than the order of
// GF(2^8).
var ErrMaxShardNum = errors.New("cannot create Encoder with more than 256 data+parity shards")

// ErrNotSupported is returned when an operation is not supported.
var ErrNotSupported = errors.New("operation not supported")

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

// buildMatrixJerasure creates the same encoding matrix as Jerasure library
//
// The top square of the matrix is guaranteed to be an identity
// matrix, which means that the data shards are unchanged after
// encoding.
func buildMatrixJerasure(dataShards, totalShards int) (matrix, error) {
	// Start with a Vandermonde matrix.  This matrix would work,
	// in theory, but doesn't have the property that the data
	// shards are unchanged after encoding.
	vm, err := vandermonde(totalShards, dataShards)
	if err != nil {
		return nil, err
	}

	// Jerasure does this:
	// first row is always 100..00
	vm[0][0] = 1
	for i := 1; i < dataShards; i++ {
		vm[0][i] = 0
	}
	// last row is always 000..01
	for i := 0; i < dataShards-1; i++ {
		vm[totalShards-1][i] = 0
	}
	vm[totalShards-1][dataShards-1] = 1

	for i := 0; i < dataShards; i++ {
		// Find the row where i'th col is not 0
		r := i
		for ; r < totalShards && vm[r][i] == 0; r++ {
		}
		if r != i {
			// Swap it with i'th row if not already
			t := vm[r]
			vm[r] = vm[i]
			vm[i] = t
		}
		// Multiply by the inverted matrix (same as vm.Multiply(vm[0:dataShards].Invert()))
		if vm[i][i] != 1 {
			// Make vm[i][i] = 1 by dividing the column by vm[i][i]
			tmp := galOneOver(vm[i][i])
			for j := 0; j < totalShards; j++ {
				vm[j][i] = galMultiply(vm[j][i], tmp)
			}
		}
		for j := 0; j < dataShards; j++ {
			// Make vm[i][j] = 0 where j != i by adding vm[i][j]*vm[.][i] to each column
			tmp := vm[i][j]
			if j != i && tmp != 0 {
				for r := 0; r < totalShards; r++ {
					vm[r][j] = galAdd(vm[r][j], galMultiply(tmp, vm[r][i]))
				}
			}
		}
	}

	// Make vm[dataShards] row all ones - divide each column j by vm[dataShards][j]
	for j := 0; j < dataShards; j++ {
		tmp := vm[dataShards][j]
		if tmp != 1 {
			tmp = galOneOver(tmp)
			for i := dataShards; i < totalShards; i++ {
				vm[i][j] = galMultiply(vm[i][j], tmp)
			}
		}
	}

	// Make vm[dataShards...totalShards-1][0] column all ones - divide each row
	for i := dataShards + 1; i < totalShards; i++ {
		tmp := vm[i][0]
		if tmp != 1 {
			tmp = galOneOver(tmp)
			for j := 0; j < dataShards; j++ {
				vm[i][j] = galMultiply(vm[i][j], tmp)
			}
		}
	}

	return vm, nil
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
// Note that the maximum number of total shards is 65536, with some
// restrictions for a total larger than 256:
//
//   - Shard sizes must be multiple of 64
//   - The methods Join/Split/Update/EncodeIdx are not supported
//
// If no options are supplied, default options are used.
func New(dataShards, parityShards int, opts ...Option) (Encoder, error) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	totShards := dataShards + parityShards
	switch {
	case o.withLeopard == leopardGF16 && parityShards > 0 || totShards > 256:
		return newFF16(dataShards, parityShards, o)
	case o.withLeopard == leopardAlways && parityShards > 0:
		return newFF8(dataShards, parityShards, o)
	}
	if totShards > 256 {
		return nil, ErrMaxShardNum
	}

	r := reedSolomon{
		dataShards:   dataShards,
		parityShards: parityShards,
		totalShards:  dataShards + parityShards,
		o:            o,
	}

	if dataShards <= 0 || parityShards < 0 {
		return nil, ErrInvShardNum
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
		r.m = make([][]byte, r.totalShards)
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
		r.m, err = buildXorMatrix(dataShards, r.totalShards)
	case r.o.useCauchy:
		r.m, err = buildMatrixCauchy(dataShards, r.totalShards)
	case r.o.usePAR1Matrix:
		r.m, err = buildMatrixPAR1(dataShards, r.totalShards)
	case r.o.useJerasureMatrix:
		r.m, err = buildMatrixJerasure(dataShards, r.totalShards)
	default:
		r.m, err = buildMatrix(dataShards, r.totalShards)
	}
	if err != nil {
		return nil, err
	}

	// Calculate what we want per round
	r.o.perRound = cpuid.CPU.Cache.L2
	if r.o.perRound < 128<<10 {
		r.o.perRound = 128 << 10
	}

	divide := parityShards + 1
	if avx2CodeGen && r.o.useAVX2 && (dataShards > maxAvx2Inputs || parityShards > maxAvx2Outputs) {
		// Base on L1 cache if we have many inputs.
		r.o.perRound = cpuid.CPU.Cache.L1D
		if r.o.perRound < 32<<10 {
			r.o.perRound = 32 << 10
		}
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

	if cpuid.CPU.ThreadsPerCore > 1 && r.o.maxGoroutines > cpuid.CPU.PhysicalCores {
		// If multiple threads per core, make sure they don't contend for cache.
		r.o.perRound /= cpuid.CPU.ThreadsPerCore
	}

	// 1 input + parity must fit in cache, and we add one more to be safer.
	r.o.perRound = r.o.perRound / divide
	// Align to 64 bytes.
	r.o.perRound = ((r.o.perRound + 63) / 64) * 64

	// Final sanity check...
	if r.o.perRound < 1<<10 {
		r.o.perRound = 1 << 10
	}

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

	if r.canGFNI(avx2CodeGenMinSize, maxAvx2Inputs, maxAvx2Outputs) && r.o.maxGoroutines > gfniCodeGenMaxGoroutines {
		r.o.maxGoroutines = gfniCodeGenMaxGoroutines
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
		sz := r.dataShards * r.parityShards * 2 * 32
		r.mPool.New = func() interface{} {
			return AllocAligned(1, sz)[0]
		}
		r.mPoolSz = sz
	}
	return &r, err
}

func (r *reedSolomon) getTmpSlice() []byte {
	return r.mPool.Get().([]byte)
}

func (r *reedSolomon) putTmpSlice(b []byte) {
	if b != nil && cap(b) >= r.mPoolSz {
		r.mPool.Put(b[:r.mPoolSz])
		return
	}
	if false {
		// Sanity check
		panic(fmt.Sprintf("got short tmp returned, want %d, got %d", r.mPoolSz, cap(b)))
	}
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
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	err := checkShards(shards, false)
	if err != nil {
		return err
	}

	// Get the slice of output buffers.
	output := shards[r.dataShards:]

	// Do the coding.
	r.codeSomeShards(r.parity, shards[0:r.dataShards], output[:r.parityShards], len(shards[0]))
	return nil
}

// EncodeIdx will add parity for a single data shard.
// Parity shards should start out zeroed. The caller must zero them before first call.
// Data shards should only be delivered once. There is no check for this.
// The parity shards will always be updated and the data shards will remain the unchanged.
func (r *reedSolomon) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	if len(parity) != r.parityShards {
		return ErrTooFewShards
	}
	if len(parity) == 0 {
		return nil
	}
	if idx < 0 || idx >= r.dataShards {
		return ErrInvShardNum
	}
	err := checkShards(parity, false)
	if err != nil {
		return err
	}
	if len(parity[0]) != len(dataShard) {
		return ErrShardSize
	}

	if avx2CodeGen && len(dataShard) >= r.o.perRound && len(parity) >= avx2CodeGenMinShards && ((pshufb && r.o.useAVX2) || r.o.useAvx512GFNI || r.o.useAvxGNFI) {
		m := make([][]byte, r.parityShards)
		for iRow := range m {
			m[iRow] = r.parity[iRow][idx : idx+1]
		}
		if r.o.useAvx512GFNI || r.o.useAvxGNFI {
			r.codeSomeShardsGFNI(m, [][]byte{dataShard}, parity, len(dataShard), false)
		} else {
			r.codeSomeShardsAVXP(m, [][]byte{dataShard}, parity, len(dataShard), false)
		}
		return nil
	}

	// Process using no goroutines for now.
	start, end := 0, r.o.perRound
	if end > len(dataShard) {
		end = len(dataShard)
	}

	for start < len(dataShard) {
		in := dataShard[start:end]
		for iRow := 0; iRow < r.parityShards; iRow++ {
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
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if len(newDatashards) != r.dataShards {
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
	for _, p := range shards[r.dataShards:] {
		if p == nil {
			return ErrInvalidInput
		}
	}

	shardSize := shardSize(shards)

	// Get the slice of output buffers.
	output := shards[r.dataShards:]

	// Do the coding.
	r.updateParityShards(r.parity, shards[0:r.dataShards], newDatashards[0:r.dataShards], output, r.parityShards, shardSize)
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

	for c := 0; c < r.dataShards; c++ {
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
			for c := 0; c < r.dataShards; c++ {
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
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}
	err := checkShards(shards, false)
	if err != nil {
		return false, err
	}

	// Slice of buffers being checked.
	toCheck := shards[r.dataShards:]

	// Do the checking.
	return r.checkSomeShards(r.parity, shards[:r.dataShards], toCheck[:r.parityShards], len(shards[0])), nil
}

func (r *reedSolomon) canAVX2C(byteCount int, inputs, outputs int) bool {
	return avx2CodeGen && pshufb && r.o.useAVX2 &&
		byteCount >= avx2CodeGenMinSize && inputs+outputs >= avx2CodeGenMinShards &&
		inputs <= maxAvx2Inputs && outputs <= maxAvx2Outputs
}

func (r *reedSolomon) canGFNI(byteCount int, inputs, outputs int) bool {
	return avx2CodeGen && (r.o.useAvx512GFNI || r.o.useAvxGNFI) &&
		byteCount >= avx2CodeGenMinSize && inputs+outputs >= avx2CodeGenMinShards &&
		inputs <= maxAvx2Inputs && outputs <= maxAvx2Outputs
}

// Multiplies a subset of rows from a coding matrix by a full set of
// input totalShards to produce some output totalShards.
// 'matrixRows' is The rows from the matrix to use.
// 'inputs' An array of byte arrays, each of which is one input shard.
// The number of inputs used is determined by the length of each matrix row.
// outputs Byte arrays where the computed totalShards are stored.
// The number of outputs computed, and the
// number of matrix rows used, is determined by
// outputCount, which is the number of outputs to compute.
func (r *reedSolomon) codeSomeShards(matrixRows, inputs, outputs [][]byte, byteCount int) {
	if len(outputs) == 0 {
		return
	}
	if byteCount > r.o.minSplitSize {
		r.codeSomeShardsP(matrixRows, inputs, outputs, byteCount)
		return
	}

	// Process using no goroutines
	start, end := 0, r.o.perRound
	if end > len(inputs[0]) {
		end = len(inputs[0])
	}
	if r.canGFNI(byteCount, len(inputs), len(outputs)) {
		var gfni [maxAvx2Inputs * maxAvx2Outputs]uint64
		m := genGFNIMatrix(matrixRows, len(inputs), 0, len(outputs), gfni[:])
		if r.o.useAvx512GFNI {
			start += galMulSlicesGFNI(m, inputs, outputs, 0, byteCount)
		} else {
			start += galMulSlicesAvxGFNI(m, inputs, outputs, 0, byteCount)
		}
		end = len(inputs[0])
	} else if r.canAVX2C(byteCount, len(inputs), len(outputs)) {
		m := genAvx2Matrix(matrixRows, len(inputs), 0, len(outputs), r.getTmpSlice())
		start += galMulSlicesAvx2(m, inputs, outputs, 0, byteCount)
		r.putTmpSlice(m)
		end = len(inputs[0])
	} else if len(inputs)+len(outputs) > avx2CodeGenMinShards && r.canAVX2C(byteCount, maxAvx2Inputs, maxAvx2Outputs) {
		var gfni [maxAvx2Inputs * maxAvx2Outputs]uint64
		end = len(inputs[0])
		inIdx := 0
		m := r.getTmpSlice()
		defer r.putTmpSlice(m)
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
				if r.o.useAvx512GFNI {
					m := genGFNIMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), gfni[:])
					if inIdx == 0 {
						start = galMulSlicesGFNI(m, inPer, outPer, 0, byteCount)
					} else {
						start = galMulSlicesGFNIXor(m, inPer, outPer, 0, byteCount)
					}
				} else if r.o.useAvxGNFI {
					m := genGFNIMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), gfni[:])
					if inIdx == 0 {
						start = galMulSlicesAvxGFNI(m, inPer, outPer, 0, byteCount)
					} else {
						start = galMulSlicesAvxGFNIXor(m, inPer, outPer, 0, byteCount)
					}
				} else {
					m = genAvx2Matrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), m)
					if inIdx == 0 {
						start = galMulSlicesAvx2(m, inPer, outPer, 0, byteCount)
					} else {
						start = galMulSlicesAvx2Xor(m, inPer, outPer, 0, byteCount)
					}
				}
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
	var gfniMatrix []uint64
	useAvx2 := r.canAVX2C(byteCount, len(inputs), len(outputs))
	useGFNI := r.canGFNI(byteCount, len(inputs), len(outputs))
	if useGFNI {
		var tmp [maxAvx2Inputs * maxAvx2Outputs]uint64
		gfniMatrix = genGFNIMatrix(matrixRows, len(inputs), 0, len(outputs), tmp[:])
	} else if useAvx2 {
		avx2Matrix = genAvx2Matrix(matrixRows, len(inputs), 0, len(outputs), r.getTmpSlice())
		defer r.putTmpSlice(avx2Matrix)
	} else if (r.o.useAvx512GFNI || r.o.useAvxGNFI) && byteCount < 10<<20 && len(inputs)+len(outputs) > avx2CodeGenMinShards &&
		r.canGFNI(byteCount/4, maxAvx2Inputs, maxAvx2Outputs) {
		// It appears there is a switchover point at around 10MB where
		// Regular processing is faster...
		r.codeSomeShardsGFNI(matrixRows, inputs, outputs, byteCount, true)
		return
	} else if r.o.useAVX2 && byteCount < 10<<20 && len(inputs)+len(outputs) > avx2CodeGenMinShards &&
		r.canAVX2C(byteCount/4, maxAvx2Inputs, maxAvx2Outputs) {
		// It appears there is a switchover point at around 10MB where
		// Regular processing is faster...
		r.codeSomeShardsAVXP(matrixRows, inputs, outputs, byteCount, true)
		return
	}

	do := byteCount / gor
	if do < r.o.minSplitSize {
		do = r.o.minSplitSize
	}

	exec := func(start, stop int) {
		if stop-start >= 64 {
			if useGFNI {
				if r.o.useAvx512GFNI {
					start += galMulSlicesGFNI(gfniMatrix, inputs, outputs, start, stop)
				} else {
					start += galMulSlicesAvxGFNI(gfniMatrix, inputs, outputs, start, stop)
				}
			} else if useAvx2 {
				start += galMulSlicesAvx2(avx2Matrix, inputs, outputs, start, stop)
			}
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
// If clear is set, the first write will overwrite the output.
func (r *reedSolomon) codeSomeShardsAVXP(matrixRows, inputs, outputs [][]byte, byteCount int, clear bool) {
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

	tmp := r.getTmpSlice()
	defer r.putTmpSlice(tmp)

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
					first:  inIdx == 0 && clear,
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
					first:  inIdx == 0 && clear,
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
		defer wg.Done()
		lstart, lstop := start, start+r.o.perRound
		if lstop > stop {
			lstop = stop
		}
		for lstart < stop {
			if lstop-lstart >= minAvx2Size {
				// Execute plan...
				var n int
				for _, p := range plan {
					if p.first {
						n = galMulSlicesAvx2(p.m, p.input, p.output, lstart, lstop)
					} else {
						n = galMulSlicesAvx2Xor(p.m, p.input, p.output, lstart, lstop)
					}
				}
				lstart += n
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
					if c == 0 && clear {
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

// Perform the same as codeSomeShards, but split the workload into
// several goroutines.
// If clear is set, the first write will overwrite the output.
func (r *reedSolomon) codeSomeShardsGFNI(matrixRows, inputs, outputs [][]byte, byteCount int, clear bool) {
	var wg sync.WaitGroup
	gor := r.o.maxGoroutines

	type state struct {
		input  [][]byte
		output [][]byte
		m      []uint64
		first  bool
	}
	// Make a plan...
	plan := make([]state, 0, ((len(inputs)+maxAvx2Inputs-1)/maxAvx2Inputs)*((len(outputs)+maxAvx2Outputs-1)/maxAvx2Outputs))

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
				m := genGFNIMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), make([]uint64, len(inPer)*len(outPer)))
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0 && clear,
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
				m := genGFNIMatrix(matrixRows[outIdx:], len(inPer), inIdx, len(outPer), make([]uint64, len(inPer)*len(outPer)))
				//fmt.Println("bytes:", len(inPer)*r.o.perRound, "out:", len(outPer)*r.o.perRound)
				plan = append(plan, state{
					input:  inPer,
					output: outPer,
					m:      m,
					first:  inIdx == 0 && clear,
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
		defer wg.Done()
		lstart, lstop := start, start+r.o.perRound
		if lstop > stop {
			lstop = stop
		}
		for lstart < stop {
			if lstop-lstart >= minAvx2Size {
				// Execute plan...
				var n int
				if r.o.useAvx512GFNI {
					for _, p := range plan {
						if p.first {
							n = galMulSlicesGFNI(p.m, p.input, p.output, lstart, lstop)
						} else {
							n = galMulSlicesGFNIXor(p.m, p.input, p.output, lstart, lstop)
						}
					}
				} else {
					for _, p := range plan {
						if p.first {
							n = galMulSlicesAvxGFNI(p.m, p.input, p.output, lstart, lstop)
						} else {
							n = galMulSlicesAvxGFNIXor(p.m, p.input, p.output, lstart, lstop)
						}
					}
				}
				lstart += n
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
					if c == 0 && clear {
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

	outputs := AllocAligned(len(toCheck), byteCount)
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

// ErrInvalidShardSize is returned if shard length doesn't meet the requirements,
// typically a multiple of N.
var ErrInvalidShardSize = errors.New("invalid shard size")

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
// The length of the array must be equal to shards.
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
// The length of the array must be equal to shards.
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

// ReconstructSome will recreate only requested shards, if possible.
//
// Given a list of shards, some of which contain data, fills in the
// shards indicated by true values in the "required" parameter.
// The length of the "required" array must be equal to either Shards or DataShards.
// If the length is equal to DataShards, the reconstruction of parity shards will be ignored.
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
	if len(required) == r.totalShards {
		return r.reconstruct(shards, false, required)
	}
	return r.reconstruct(shards, true, required)
}

// reconstruct will recreate the missing data totalShards, and unless
// dataOnly is true, also the missing parity totalShards
//
// The length of "shards" array must be equal to totalShards.
// You indicate that a shard is missing by setting it to nil.
//
// If there are too few totalShards to reconstruct the missing
// ones, ErrTooFewShards will be returned.
func (r *reedSolomon) reconstruct(shards [][]byte, dataOnly bool, required []bool) error {
	if len(shards) != r.totalShards || required != nil && len(required) < r.dataShards {
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
	for i := 0; i < r.totalShards; i++ {
		if len(shards[i]) != 0 {
			numberPresent++
			if i < r.dataShards {
				dataPresent++
			}
		} else if required != nil && required[i] {
			missingRequired++
		}
	}
	if numberPresent == r.totalShards || dataOnly && dataPresent == r.dataShards ||
		required != nil && missingRequired == 0 {
		// Cool. All of the shards have data. We don't
		// need to do anything.
		return nil
	}

	// More complete sanity check
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	// Pull out an array holding just the shards that
	// correspond to the rows of the submatrix.  These shards
	// will be the input to the decoding process that re-creates
	// the missing data shards.
	//
	// Also, create an array of indices of the valid rows we do have
	// and the invalid rows we don't have up until we have enough valid rows.
	subShards := make([][]byte, r.dataShards)
	validIndices := make([]int, r.dataShards)
	invalidIndices := make([]int, 0)
	subMatrixRow := 0
	for matrixRow := 0; matrixRow < r.totalShards && subMatrixRow < r.dataShards; matrixRow++ {
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
		subMatrix, _ := newMatrix(r.dataShards, r.dataShards)
		for subMatrixRow, validIndex := range validIndices {
			for c := 0; c < r.dataShards; c++ {
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
		err = r.tree.InsertInvertedMatrix(invalidIndices, dataDecodeMatrix, r.totalShards)
		if err != nil {
			return err
		}
	}

	// Re-create any data shards that were missing.
	//
	// The input to the coding is all of the shards we actually
	// have, and the output is the missing data shards.  The computation
	// is done using the special decode matrix we just built.
	outputs := make([][]byte, r.parityShards)
	matrixRows := make([][]byte, r.parityShards)
	outputCount := 0

	for iShard := 0; iShard < r.dataShards; iShard++ {
		if len(shards[iShard]) == 0 && (required == nil || required[iShard]) {
			if cap(shards[iShard]) >= shardSize {
				shards[iShard] = shards[iShard][0:shardSize]
			} else {
				shards[iShard] = AllocAligned(1, shardSize)[0]
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
	for iShard := r.dataShards; iShard < r.totalShards; iShard++ {
		if len(shards[iShard]) == 0 && (required == nil || required[iShard]) {
			if cap(shards[iShard]) >= shardSize {
				shards[iShard] = shards[iShard][0:shardSize]
			} else {
				shards[iShard] = AllocAligned(1, shardSize)[0]
			}
			outputs[outputCount] = shards[iShard]
			matrixRows[outputCount] = r.parity[iShard-r.dataShards]
			outputCount++
		}
	}
	r.codeSomeShards(matrixRows, shards[:r.dataShards], outputs[:outputCount], shardSize)
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
// If there is extra capacity on the provided data slice
// it will be used instead of allocating parity shards.
// It will be zeroed out.
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
	if r.totalShards == 1 {
		return [][]byte{data}, nil
	}

	dataLen := len(data)
	// Calculate number of bytes per data shard.
	perShard := (len(data) + r.dataShards - 1) / r.dataShards
	needTotal := r.totalShards * perShard

	if cap(data) > len(data) {
		if cap(data) > needTotal {
			data = data[:needTotal]
		} else {
			data = data[:cap(data)]
		}
		clear := data[dataLen:]
		for i := range clear {
			clear[i] = 0
		}
	}

	// Only allocate memory if necessary
	var padding [][]byte
	if len(data) < needTotal {
		// calculate maximum number of full shards in `data` slice
		fullShards := len(data) / perShard
		padding = AllocAligned(r.totalShards-fullShards, perShard)

		if dataLen > perShard*fullShards {
			// Copy partial shards
			copyFrom := data[perShard*fullShards : dataLen]
			for i := range padding {
				if len(copyFrom) == 0 {
					break
				}
				copyFrom = copyFrom[copy(padding[i], copyFrom):]
			}
		}
	}

	// Split into equal-length shards.
	dst := make([][]byte, r.totalShards)
	i := 0
	for ; i < len(dst) && len(data) >= perShard; i++ {
		dst[i] = data[:perShard:perShard]
		data = data[perShard:]
	}

	for j := 0; i+j < len(dst); j++ {
		dst[i+j] = padding[0]
		padding = padding[1:]
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
	if len(shards) < r.dataShards {
		return ErrTooFewShards
	}
	shards = shards[:r.dataShards]

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
