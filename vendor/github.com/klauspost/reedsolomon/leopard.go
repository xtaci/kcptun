package reedsolomon

// This is a O(n*log n) implementation of Reed-Solomon
// codes, ported from the C++ library https://github.com/catid/leopard.
//
// The implementation is based on the paper
//
// S.-J. Lin, T. Y. Al-Naffouri, Y. S. Han, and W.-H. Chung,
// "Novel Polynomial Basis with Fast Fourier Transform
// and Its Application to Reed-Solomon Erasure Codes"
// IEEE Trans. on Information Theory, pp. 6284-6299, November, 2016.

import (
	"bytes"
	"io"
	"math/bits"
	"sync"
	"unsafe"

	"github.com/klauspost/cpuid/v2"
)

const maxWorkSize16 = 64 << 10 // Cap for GF16 chunk size.
const maxZeroBufferSize16 = maxWorkSize16

var zeroBufferPool16Once sync.Once
var zeroBufferPool16Buf *[maxZeroBufferSize16]byte
var zeroBufferPool16 = func() *[maxZeroBufferSize16]byte {
	zeroBufferPool16Once.Do(func() {
		zeroBufferPool16Buf = &[maxZeroBufferSize16]byte{}
	})
	return zeroBufferPool16Buf
}

// leopardFF16 is like reedSolomon but for more than 256 total shards.
type leopardFF16 struct {
	dataShards   int // Number of data shards, should not be modified.
	parityShards int // Number of parity shards, should not be modified.
	totalShards  int // Total number of shards. Calculated, and should not be modified.

	workAlloc WorkAllocator

	// Pools for slice header arrays, avoiding per-call allocations.
	shardSlicePool sync.Pool // [][]byte of len totalShards
	workSlicePool  sync.Pool // [][]byte — sized at first use

	o options
}

// newFF16 is like New, but for more than 256 total shards.
func newFF16(dataShards, parityShards int, opt options) (*leopardFF16, error) {
	initConstants()

	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvShardNum
	}

	if dataShards+parityShards > 65536 {
		return nil, ErrMaxShardNum
	}

	if !leopardFits(dataShards, parityShards, order) {
		return nil, ErrInvShardCombo
	}

	r := &leopardFF16{
		dataShards:   dataShards,
		parityShards: parityShards,
		totalShards:  dataShards + parityShards,
		workAlloc:    opt.workAlloc,
		o:            opt,
	}
	return r, nil
}

var _ = Extensions(&leopardFF16{})

func (r *leopardFF16) ShardSizeMultiple() int {
	return 64
}

func (r *leopardFF16) DataShards() int {
	return r.dataShards
}

func (r *leopardFF16) ParityShards() int {
	return r.parityShards
}

func (r *leopardFF16) TotalShards() int {
	return r.totalShards
}

func (r *leopardFF16) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each)
}

func (r *leopardFF16) DecodeIdx(dst [][]byte, expectInput []bool, input [][]byte) error {
	return ErrNotSupported
}

type ffe uint16

const (
	bitwidth   = 16
	order      = 1 << bitwidth
	modulus    = order - 1
	polynomial = 0x1002D
)

var (
	fftSkew  *[modulus]ffe
	logWalsh *[order]ffe
)

// Logarithm Tables
var (
	logLUT *[order]ffe
	expLUT *[order]ffe
)

// Stores the partial products of x * y at offset x + y * 65536
// Repeated accesses from the same y value are faster
var mul16LUTs *[order]mul16LUT

type mul16LUT struct {
	// Contains Lo product as a single lookup.
	// Should be XORed with Hi lookup for result.
	Lo [256]ffe
	Hi [256]ffe
}

// Stores lookup for avx2
var multiply256LUT *[order][8 * 16]byte

// Stores GFNI transformation matrices for GF16.
// Each entry contains 4 uint64 matrices [A, B, C, D] representing:
//
//	[out_lo]   [A  B] [in_lo]
//	[out_hi] = [C  D] [in_hi]
var gf2p811dMulMatrices16 *[order][4]uint64

func (r *leopardFF16) Encode(shards [][]byte) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, false); err != nil {
		return err
	}
	return r.encode(shards)
}

// encodeChunkSize returns the chunk size for the given shard size and
// number of work buffers. It tries to keep the working set in L3 cache,
// capped at maxWorkSize16.
func encodeChunkSize(shardSize, workBufs int) int {
	l3 := cpuid.CPU.Cache.L3
	if l3 <= 0 {
		// Assume 16MB L3 if not detected.
		l3 = 16 << 20
	}
	// Target: workBufs * chunkSize fits in ~2/3 of L3.
	chunkSize := l3 * 2 / (workBufs * 3)
	chunkSize = min(chunkSize, maxWorkSize16)
	chunkSize &^= 63 // 64-byte alignment
	if chunkSize < 4<<10 {
		chunkSize = 4 << 10
	}
	return min(chunkSize, shardSize)
}

func (r *leopardFF16) encode(shards [][]byte) error {
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	m := ceilPow2(r.parityShards)
	mtrunc := min(r.dataShards, m)
	lastCount := r.dataShards % m
	chunkSize := encodeChunkSize(shardSize, m*2)

	work, err := getWork(r.workAlloc, m*2, chunkSize)
	if err != nil {
		return err
	}
	defer r.workAlloc.Put(work)

	skewLUT := fftSkew[m-1:]

	sh := r.getShardSlice()
	defer r.putShardSlice(sh)
	wMod := r.getWorkSlice(len(work))
	defer r.putWorkSlice(wMod)
	copy(wMod, work)
	for off := 0; off < shardSize; off += chunkSize {
		work := wMod
		sh := sh
		end := off + chunkSize
		if end > shardSize {
			end = shardSize
			sz := end - off
			for i := range work {
				work[i] = work[i][:sz]
			}
		}
		for i := range shards {
			sh[i] = shards[i][off:end]
		}

		// Write parity directly into output slices.
		res := shards[r.dataShards:r.totalShards]
		for i := range res {
			work[i] = res[i][off:end]
		}

		r.encodeChunk(sh[:r.dataShards], sh, mtrunc, lastCount, m, work, skewLUT)
	}
	return nil
}

func (r *leopardFF16) encodeChunk(data [][]byte, sh [][]byte, mtrunc, lastCount, m int, work [][]byte, skewLUT []ffe) {
	ifftDITEncoder(
		data,
		mtrunc,
		work,
		nil,
		m,
		skewLUT,
		&r.o,
	)

	if m >= r.dataShards {
		goto skip_body
	}

	{
		sh := sh
		skewLUT := skewLUT
		// For sets of m data pieces:
		for i := m; i+m <= r.dataShards; i += m {
			sh = sh[m:]
			skewLUT = skewLUT[m:]

			// work <- work xor IFFT(data + i, m, m + i)
			ifftDITEncoder(
				sh,
				m,
				work[m:],
				work,
				m,
				skewLUT,
				&r.o,
			)
		}

		// Handle final partial set of m pieces:
		if lastCount != 0 {
			sh = sh[m:]
			skewLUT = skewLUT[m:]

			ifftDITEncoder(
				sh,
				lastCount,
				work[m:],
				work,
				m,
				skewLUT,
				&r.o,
			)
		}
	}

skip_body:
	fftDIT(work, r.parityShards, m, fftSkew[:], &r.o)
}

func (r *leopardFF16) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	return ErrNotSupported
}

func (r *leopardFF16) Join(dst io.Writer, shards [][]byte, outSize int) error {
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

func (r *leopardFF16) Update(shards [][]byte, newDatashards [][]byte) error {
	return ErrNotSupported
}

func (r *leopardFF16) Split(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, ErrShortData
	}
	if r.totalShards == 1 && len(data)&63 == 0 {
		return [][]byte{data}, nil
	}
	dataLen := len(data)
	// Calculate number of bytes per data shard.
	perShard := (len(data) + r.dataShards - 1) / r.dataShards
	perShard = ((perShard + 63) / 64) * 64
	needTotal := r.totalShards * perShard

	if cap(data) > len(data) {
		if cap(data) > needTotal {
			data = data[:needTotal]
		} else {
			data = data[:cap(data)]
		}
		clear(data[dataLen:])
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
	} else {
		clear(data[dataLen : r.totalShards*perShard])
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

func (r *leopardFF16) ReconstructSome(shards [][]byte, required []bool) error {
	if len(required) == r.totalShards {
		return r.reconstruct(shards, true)
	}
	return r.reconstruct(shards, false)
}

func (r *leopardFF16) Reconstruct(shards [][]byte) error {
	return r.reconstruct(shards, true)
}

func (r *leopardFF16) ReconstructData(shards [][]byte) error {
	return r.reconstruct(shards, false)
}

func (r *leopardFF16) Verify(shards [][]byte) (bool, error) {
	if len(shards) != r.totalShards {
		return false, ErrTooFewShards
	}
	if err := checkShards(shards, false); err != nil {
		return false, err
	}

	// Re-encode parity shards to temporary storage.
	shardSize := len(shards[0])
	outputs := make([][]byte, r.totalShards)
	copy(outputs, shards[:r.dataShards])
	for i := r.dataShards; i < r.totalShards; i++ {
		outputs[i] = make([]byte, shardSize)
	}
	if err := r.Encode(outputs); err != nil {
		return false, err
	}

	// Compare.
	for i := r.dataShards; i < r.totalShards; i++ {
		if !bytes.Equal(outputs[i], shards[i]) {
			return false, nil
		}
	}
	return true, nil
}

func (r *leopardFF16) reconstruct(shards [][]byte, recoverAll bool) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, true); err != nil {
		return err
	}

	// Quick check: are all of the shards present?  If so, there's
	// nothing to do.
	numberPresent := 0
	dataPresent := 0
	for i := 0; i < r.totalShards; i++ {
		if len(shards[i]) != 0 {
			numberPresent++
			if i < r.dataShards {
				dataPresent++
			}
		}
	}
	if numberPresent == r.totalShards || !recoverAll && dataPresent == r.dataShards {
		// Cool. All of the shards have data. We don't
		// need to do anything.
		return nil
	}

	// Use only if the requested reconstruction set is sparse. ReconstructData
	// ignores missing parity shards, so gating this on total missing shards
	// disables the sparse FFT path for data-only recovery with many absent
	// parity shards. Data-only recovery computes at most dataShards outputs
	// from m+dataShards possible FFT outputs, so use the sparse path there.
	useBits := !recoverAll || r.totalShards-numberPresent <= r.parityShards/4

	// Check if we have enough to reconstruct.
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	m := ceilPow2(r.parityShards)
	n := ceilPow2(m + r.dataShards)

	const LEO_ERROR_BITFIELD_OPT = true

	// Fill in error locations.
	var errorBits errorBitfield
	var errLocs [order]ffe
	inputCount := r.parityShards
	for i := 0; i < r.parityShards; i++ {
		if len(shards[i+r.dataShards]) == 0 {
			errLocs[i] = 1
			if LEO_ERROR_BITFIELD_OPT && recoverAll {
				errorBits.set(i)
			}
		}
	}
	for i := r.parityShards; i < m; i++ {
		errLocs[i] = 1
		if LEO_ERROR_BITFIELD_OPT && recoverAll {
			errorBits.set(i)
		}
	}
	for i := 0; i < r.dataShards; i++ {
		if len(shards[i]) == 0 {
			errLocs[i+m] = 1
			if LEO_ERROR_BITFIELD_OPT {
				errorBits.set(i + m)
			}
		} else {
			inputCount = m + i + 1
		}
	}

	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.prepare(n)
	}

	// Evaluate error locator polynomial
	fwht(&errLocs, m+r.dataShards)

	for i := range order {
		errLocs[i] = ffe((uint(errLocs[i]) * uint(logWalsh[i])) % modulus)
	}

	fwht(&errLocs, order)

	chunkSize := encodeChunkSize(shardSize, n)

	work, err := getWork(r.workAlloc, n, chunkSize)
	if err != nil {
		return err
	}
	defer r.workAlloc.Put(work)

	// sh preserves the original nil entries so reconstructChunk can
	// distinguish present vs missing shards after pre-allocation.
	sh := r.getShardSlice()
	defer r.putShardSlice(sh)
	copy(sh, shards)

	// Pre-allocate missing output shards.
	end := r.dataShards
	if recoverAll {
		end = r.totalShards
	}
	for i := 0; i < end; i++ {
		if len(shards[i]) != 0 {
			continue
		}
		if cap(shards[i]) >= shardSize {
			shards[i] = shards[i][:shardSize]
		} else {
			shards[i] = make([]byte, shardSize)
		}
	}

	if chunkSize >= shardSize {
		r.reconstructChunk(sh, shards, work, m, n, inputCount, &errLocs, useBits, &errorBits, recoverAll)
		return nil
	}

	// Process in cache-friendly chunks.
	wMod := r.getWorkSlice(len(work))
	defer r.putWorkSlice(wMod)
	copy(wMod, work)
	shChunk := r.getShardSlice()
	defer r.putShardSlice(shChunk)
	copy(shChunk, sh)
	outChunk := r.getShardSlice()
	defer r.putShardSlice(outChunk)
	for off := 0; off < shardSize; off += chunkSize {
		work := wMod
		shChunk := shChunk
		outChunk := outChunk
		endSlice := off + chunkSize
		if endSlice > shardSize {
			endSlice = shardSize
			sz := endSlice - off
			for i := range work {
				work[i] = work[i][:sz]
			}
		}
		for i := range shards {
			if len(sh[i]) != 0 {
				shChunk[i] = shards[i][off:endSlice]
			}
			if len(shards[i]) != 0 {
				outChunk[i] = shards[i][off:endSlice]
			}
		}

		r.reconstructChunk(shChunk, outChunk, work, m, n, inputCount, &errLocs, useBits, &errorBits, recoverAll)
	}
	return nil
}

// reconstructChunk processes one chunk of the reconstruct pipeline.
// sh has nil entries for missing shards (used for presence checks).
// out has allocated entries for all shards (used for writing output).
func (r *leopardFF16) reconstructChunk(sh, out [][]byte, work [][]byte, m, n, inputCount int, errLocs *[order]ffe, useBits bool, errorBits *errorBitfield, recoverAll bool) {
	const LEO_ERROR_BITFIELD_OPT = true
	outputCount := m + r.dataShards

	// work <- recovery data
	for i := 0; i < r.parityShards; i++ {
		if len(sh[i+r.dataShards]) != 0 {
			mulgf16(work[i], sh[i+r.dataShards], errLocs[i], &r.o)
		} else {
			clear(work[i])
		}
	}
	for i := r.parityShards; i < m; i++ {
		clear(work[i])
	}

	// work <- original data
	for i := 0; i < r.dataShards; i++ {
		if len(sh[i]) != 0 {
			mulgf16(work[m+i], sh[i], errLocs[m+i], &r.o)
		} else {
			clear(work[m+i])
		}
	}
	for i := m + r.dataShards; i < n; i++ {
		clear(work[i])
	}

	// work <- IFFT(work, n, 0)
	ifftDITDecoder(
		inputCount,
		work,
		n,
		fftSkew[:],
		&r.o,
	)

	// work <- FormalDerivative(work, n)
	for i := 1; i < n; i++ {
		width := ((i ^ (i - 1)) + 1) >> 1
		slicesXor(work[i-width:i], work[i:i+width], &r.o)
	}

	// work <- FFT(work, n, 0) truncated to m + dataShards
	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.fftDIT(work, outputCount, n, fftSkew[:], &r.o)
	} else {
		fftDIT(work, outputCount, n, fftSkew[:], &r.o)
	}

	// Reveal erasures
	end := r.dataShards
	if recoverAll {
		end = r.totalShards
	}
	for i := 0; i < end; i++ {
		if len(sh[i]) != 0 {
			continue
		}
		if i >= r.dataShards {
			mulgf16(out[i], work[i-r.dataShards], modulus-errLocs[i-r.dataShards], &r.o)
		} else {
			mulgf16(out[i], work[i+m], modulus-errLocs[i+m], &r.o)
		}
	}
}

// Basic no-frills version for decoder
func ifftDITDecoder(mtrunc int, work [][]byte, m int, skewLUT []ffe, o *options) {
	// Decimation in time: Unroll 2 layers at a time
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend-1]
			log_m02 := skewLUT[iend+dist-1]
			log_m23 := skewLUT[iend+dist*2-1]

			// For each set of dist elements:
			for i := r; i < iend; i++ {
				ifftDIT4(work[i:], dist, log_m01, log_m23, log_m02, o)
			}
		}
		dist = dist4
		dist4 <<= 2
	}

	// If there is one layer left:
	if dist < m {
		// Assuming that dist = m / 2
		if dist*2 != m {
			panic("internal error")
		}

		log_m := skewLUT[dist-1]

		if log_m == modulus {
			slicesXor(work[dist:2*dist], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT2(
					work[i],
					work[i+dist],
					log_m,
					o,
				)
			}
		}
	}
}

// In-place FFT for encoder and decoder
func fftDIT(work [][]byte, mtrunc, m int, skewLUT []ffe, o *options) {
	// Decimation in time: Unroll 2 layers at a time
	dist4 := m
	dist := m >> 2
	for dist != 0 {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend-1]
			log_m02 := skewLUT[iend+dist-1]
			log_m23 := skewLUT[iend+dist*2-1]

			// For each set of dist elements:
			for i := r; i < iend; i++ {
				fftDIT4(
					work[i:],
					dist,
					log_m01,
					log_m23,
					log_m02,
					o,
				)
			}
		}
		dist4 = dist
		dist >>= 2
	}

	// If there is one layer left:
	if dist4 == 2 {
		for r := 0; r < mtrunc; r += 2 {
			log_m := skewLUT[r+1-1]

			if log_m == modulus {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT2(work[r], work[r+1], log_m, o)
			}
		}
	}
}

// 4-way butterfly
func fftDIT4Ref(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	// First layer:
	if log_m02 == modulus {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		fftDIT2(work[0], work[dist*2], log_m02, o)
		fftDIT2(work[dist], work[dist*3], log_m02, o)
	}

	// Second layer:
	if log_m01 == modulus {
		sliceXor(work[0], work[dist], o)
	} else {
		fftDIT2(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		fftDIT2(work[dist*2], work[dist*3], log_m23, o)
	}
}

// Unrolled IFFT for encoder
func ifftDITEncoder(data [][]byte, mtrunc int, work [][]byte, xorRes [][]byte, m int, skewLUT []ffe, o *options) {
	dist := 1
	dist4 := 4

	if dist4 <= m {
		shardSize := len(data[0])
		canUseFused := (o.useAvx512GFNI || o.useAvxGNFI || o.useAVX512 || o.useAVX2) && shardSize <= maxZeroBufferSize16

		if canUseFused {
			// SIMD path: fuse copy with first layer butterfly
			fullGroups := mtrunc &^ 3
			zb := zeroBufferPool16()[:shardSize]

			for r := 0; r < fullGroups; r += dist4 {
				iend := r + dist
				log_m01 := skewLUT[iend]
				log_m02 := skewLUT[iend+dist]
				log_m23 := skewLUT[iend+dist*2]
				ifftDIT4Dst(work[r:], data[r:], dist, log_m01, log_m23, log_m02, o)
			}

			if fullGroups < mtrunc {
				r := fullGroups
				iend := r + dist
				log_m01 := skewLUT[iend]
				log_m02 := skewLUT[iend+dist]
				log_m23 := skewLUT[iend+dist*2]

				src := [4][]byte{zb, zb, zb, zb}
				for i := 0; i < mtrunc-fullGroups; i++ {
					src[i] = data[fullGroups+i]
				}
				ifftDIT4Dst(work[r:], src[:], dist, log_m01, log_m23, log_m02, o)
			}

			clearStart := (mtrunc + 3) &^ 3
			for i := clearStart; i < m; i++ {
				clear(work[i])
			}
		} else {
			// Non-SIMD or large shards: original approach
			for i := range mtrunc {
				copy(work[i], data[i])
			}
			for i := mtrunc; i < m; i++ {
				clear(work[i])
			}

			for r := 0; r < mtrunc; r += dist4 {
				iend := r + dist
				log_m01 := skewLUT[iend]
				log_m02 := skewLUT[iend+dist]
				log_m23 := skewLUT[iend+dist*2]
				for i := r; i < iend; i++ {
					ifftDIT4(work[i:], dist, log_m01, log_m23, log_m02, o)
				}
			}
		}

		dist = dist4
		dist4 <<= 2

		// Subsequent layers: in-place (same for both paths)
		for dist4 <= m {
			for r := 0; r < mtrunc; r += dist4 {
				iend := r + dist
				log_m01 := skewLUT[iend]
				log_m02 := skewLUT[iend+dist]
				log_m23 := skewLUT[iend+dist*2]
				for i := r; i < iend; i++ {
					ifftDIT4(work[i:], dist, log_m01, log_m23, log_m02, o)
				}
			}
			dist = dist4
			dist4 <<= 2
		}
	} else {
		// m < 4: fallback
		for i := range mtrunc {
			copy(work[i], data[i])
		}
		for i := mtrunc; i < m; i++ {
			clear(work[i])
		}
	}

	// If there is one layer left:
	if dist < m {
		// Assuming that dist = m / 2
		if dist*2 != m {
			panic("internal error")
		}

		logm := skewLUT[dist]

		if logm == modulus {
			slicesXor(work[dist:dist*2], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT2(work[i], work[i+dist], logm, o)
			}
		}
	}

	// I tried unrolling this but it does not provide more than 5% performance
	// improvement for 16-bit finite fields, so it's not worth the complexity.
	if xorRes != nil {
		slicesXor(xorRes[:m], work[:m], o)
	}
}

func ifftDIT4Ref(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	// First layer:
	if log_m01 == modulus {
		sliceXor(work[0], work[dist], o)
	} else {
		ifftDIT2(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		ifftDIT2(work[dist*2], work[dist*3], log_m23, o)
	}

	// Second layer:
	if log_m02 == modulus {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		ifftDIT2(work[0], work[dist*2], log_m02, o)
		ifftDIT2(work[dist], work[dist*3], log_m02, o)
	}
}

func ifftDIT4DstRef(dst, work [][]byte, dist int, log_m01, log_m23, log_m02 ffe, o *options) {
	for i := range 4 {
		copy(dst[i*dist], work[i*dist])
	}
	ifftDIT4Ref(dst, dist, log_m01, log_m23, log_m02, o)
}

// Reference version of muladd: x[] ^= y[] * log_m
func refMulAdd(x, y []byte, log_m ffe) {
	lut := &mul16LUTs[log_m]

	for len(x) >= 64 {
		// Assert sizes for no bounds checks in loop
		hiA := y[32:64]
		loA := y[:32]
		dst := x[:64] // Needed, but not checked...
		for i, lo := range loA {
			hi := hiA[i]
			prod := lut.Lo[lo] ^ lut.Hi[hi]

			dst[i] ^= byte(prod)
			dst[i+32] ^= byte(prod >> 8)
		}
		x = x[64:]
		y = y[64:]
	}
}

// refMulAdd8x performs 8 mul-add accumulates sharing the same input,
// reading in once per 64-byte chunk. Pure Go fallback for mulgf16Xor8.
func refMulAdd8x(scalars *[8]uint16, in []byte, outs *[8][]byte) {
	var luts [8]*mul16LUT
	for k, c := range scalars {
		if c != 0 {
			luts[k] = &mul16LUTs[logLUT[ffe(c)]]
		}
	}
	out := *outs // local copy so we can re-slice without mutating caller
	for len(in) >= 64 {
		hiIn := in[32:64]
		loIn := in[:32]
		for i, lo := range loIn {
			hi := hiIn[i]
			for k := range 8 {
				if luts[k] == nil {
					continue
				}
				prod := luts[k].Lo[lo] ^ luts[k].Hi[hi]
				out[k][i] ^= byte(prod)
				out[k][i+32] ^= byte(prod >> 8)
			}
		}
		in = in[64:]
		for k := range out {
			out[k] = out[k][64:]
		}
	}
}

// slicesXor calls xor for every slice pair in v1, v2.
func slicesXor(v1, v2 [][]byte, o *options) {
	for i, v := range v1 {
		sliceXor(v2[i], v, o)
	}
}

// Reference version of mul: x[] = y[] * log_m
func refMul(x, y []byte, log_m ffe) {
	lut := &mul16LUTs[log_m]

	for off := 0; off < len(x); off += 64 {
		loA := y[off : off+32]
		hiA := y[off+32:]
		hiA = hiA[:len(loA)]
		for i, lo := range loA {
			hi := hiA[i]
			prod := lut.Lo[lo] ^ lut.Hi[hi]

			x[off+i] = byte(prod)
			x[off+i+32] = byte(prod >> 8)
		}
	}
}

// Returns a * Log(b)
func mulLog(a, log_b ffe) ffe {
	/*
	   Note that this operation is not a normal multiplication in a finite
	   field because the right operand is already a logarithm.  This is done
	   because it moves K table lookups from the Decode() method into the
	   initialization step that is less performance critical.  The LogWalsh[]
	   table below contains precalculated logarithms so it is easier to do
	   all the other multiplies in that form as well.
	*/
	if a == 0 {
		return 0
	}
	return expLUT[addMod(logLUT[a], log_b)]
}

// addMod returns the sum of a and b modulo modulus. This can return modulus but
// it is expected that callers will convert modulus to 0.
func addMod(a, b ffe) ffe {
	sum := uint(a) + uint(b)

	// Partial reduction step which allows for modulus to be returned.
	return ffe(sum + sum>>bitwidth)
}

// z = x - y (mod kModulus)
func subMod(a, b ffe) ffe {
	dif := uint(a) - uint(b)

	// Partial reduction step, allowing for kModulus to be returned
	return ffe(dif + dif>>bitwidth)
}

// ceilPow2 returns power of two at or above n.
func ceilPow2(n int) int {
	const w = int(unsafe.Sizeof(n) * 8)
	return 1 << (w - bits.LeadingZeros(uint(n-1)))
}

// Decimation in time (DIT) Fast Walsh-Hadamard Transform
// Unrolls pairs of layers to perform cross-layer operations in registers
// mtrunc: Number of elements that are non-zero at the front of data
func fwht(data *[order]ffe, mtrunc int) {
	// Decimation in time: Unroll 2 layers at a time
	dist := 1
	dist4 := 4
	for dist4 <= order {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			// For each set of dist elements:
			// Use 16 bit indices to avoid bounds check on [65536]ffe.
			dist := uint16(dist)
			off := uint16(r)
			for range dist {
				// fwht4(data[i:], dist) inlined...
				// Reading values appear faster than updating pointers.
				// Casting to uint is not faster.
				t0 := data[off]
				t1 := data[off+dist]
				t2 := data[off+dist*2]
				t3 := data[off+dist*3]

				t0, t1 = fwht2alt(t0, t1)
				t2, t3 = fwht2alt(t2, t3)
				t0, t2 = fwht2alt(t0, t2)
				t1, t3 = fwht2alt(t1, t3)

				data[off] = t0
				data[off+dist] = t1
				data[off+dist*2] = t2
				data[off+dist*3] = t3
				off++
			}
		}
		dist = dist4
		dist4 <<= 2
	}
}

func fwht4(data []ffe, s int) {
	s2 := s << 1

	t0 := &data[0]
	t1 := &data[s]
	t2 := &data[s2]
	t3 := &data[s2+s]

	fwht2(t0, t1)
	fwht2(t2, t3)
	fwht2(t0, t2)
	fwht2(t1, t3)
}

// {a, b} = {a + b, a - b} (Mod Q)
func fwht2(a, b *ffe) {
	sum := addMod(*a, *b)
	dif := subMod(*a, *b)
	*a = sum
	*b = dif
}

// fwht2alt is as fwht2, but returns result.
func fwht2alt(a, b ffe) (ffe, ffe) {
	sum := uint(a) + uint(b)
	dif := uint(a) - uint(b)
	sum = sum + sum>>bitwidth
	dif = dif + dif>>bitwidth
	return ffe(sum), ffe(dif)
}

var initOnce sync.Once

func initConstants() {
	initOnce.Do(func() {
		initLUTs()
		initFFTSkew()
		initMul16LUT()
	})
}

// Initialize logLUT, expLUT.
func initLUTs() {
	cantorBasis := [bitwidth]ffe{
		0x0001, 0xACCA, 0x3C0E, 0x163E,
		0xC582, 0xED2E, 0x914C, 0x4012,
		0x6C98, 0x10D8, 0x6A72, 0xB900,
		0xFDB8, 0xFB34, 0xFF38, 0x991E,
	}

	expLUT = &[order]ffe{}
	logLUT = &[order]ffe{}

	// LFSR table generation:
	state := 1
	for i := range ffe(modulus) {
		expLUT[state] = i
		state <<= 1
		if state >= order {
			state ^= polynomial
		}
	}
	expLUT[0] = modulus

	// Conversion to Cantor basis:

	logLUT[0] = 0
	for i := range bitwidth {
		basis := cantorBasis[i]
		width := 1 << i

		for j := range width {
			logLUT[j+width] = logLUT[j] ^ basis
		}
	}

	for i := range order {
		logLUT[i] = expLUT[logLUT[i]]
	}

	for i := range order {
		expLUT[logLUT[i]] = ffe(i)
	}

	expLUT[modulus] = expLUT[0]
}

// Initialize fftSkew.
func initFFTSkew() {
	var temp [bitwidth - 1]ffe

	// Generate FFT skew vector {1}:

	for i := 1; i < bitwidth; i++ {
		temp[i-1] = ffe(1 << i)
	}

	fftSkew = &[modulus]ffe{}
	logWalsh = &[order]ffe{}

	for m := range bitwidth - 1 {
		step := 1 << (m + 1)

		fftSkew[1<<m-1] = 0

		for i := m; i < bitwidth-1; i++ {
			s := 1 << (i + 1)

			for j := 1<<m - 1; j < s; j += step {
				fftSkew[j+s] = fftSkew[j] ^ temp[i]
			}
		}

		temp[m] = modulus - logLUT[mulLog(temp[m], logLUT[temp[m]^1])]

		for i := m + 1; i < bitwidth-1; i++ {
			sum := addMod(logLUT[temp[i]^1], temp[m])
			temp[i] = mulLog(temp[i], sum)
		}
	}

	for i := range modulus {
		fftSkew[i] = logLUT[fftSkew[i]]
	}

	// Precalculate FWHT(Log[i]):

	for i := range order {
		logWalsh[i] = logLUT[i]
	}
	logWalsh[0] = 0

	fwht(logWalsh, order)
}

func initMul16LUT() {
	mul16LUTs = &[order]mul16LUT{}

	// For each log_m multiplicand:
	for log_m := range order {
		var tmp [64]ffe
		for nibble, shift := 0, 0; nibble < 4; {
			nibble_lut := tmp[nibble*16:]

			for xnibble := range 16 {
				prod := mulLog(ffe(xnibble<<shift), ffe(log_m))
				nibble_lut[xnibble] = prod
			}
			nibble++
			shift += 4
		}
		lut := &mul16LUTs[log_m]
		for i := range lut.Lo[:] {
			lut.Lo[i] = tmp[i&15] ^ tmp[((i>>4)+16)]
			lut.Hi[i] = tmp[((i&15)+32)] ^ tmp[((i>>4)+48)]
		}
	}
	if cpuid.CPU.Has(cpuid.SSSE3) || cpuid.CPU.Has(cpuid.AVX2) || cpuid.CPU.Has(cpuid.AVX512F) {
		multiply256LUT = &[order][16 * 8]byte{}

		for logM := range multiply256LUT[:] {
			// For each 4 bits of the finite field width in bits:
			shift := 0
			for i := range 4 {
				// Construct 16 entry LUT for PSHUFB
				prodLo := multiply256LUT[logM][i*16 : i*16+16]
				prodHi := multiply256LUT[logM][4*16+i*16 : 4*16+i*16+16]
				for x := range prodLo[:] {
					prod := mulLog(ffe(x<<shift), ffe(logM))
					prodLo[x] = byte(prod)
					prodHi[x] = byte(prod >> 8)
				}
				shift += 4
			}
		}
	}
	if cpuid.CPU.Has(cpuid.GFNI) {
		gf2p811dMulMatrices16 = &[order][4]uint64{}
		for logM := range gf2p811dMulMatrices16[:] {
			var A, B, C, D uint64

			// A, C: effect of input lo_byte bits on output
			// VGF2P8AFFINEQB accesses byte (7-j) for output bit j, so we need:
			// position = (7-outputBit)*8 + inputBit
			for inputBit := range 8 {
				result := mulLog(ffe(1<<inputBit), ffe(logM))
				for outputBit := range 8 {
					if (byte(result)>>outputBit)&1 == 1 {
						A |= 1 << ((7-outputBit)*8 + inputBit)
					}
					if (byte(result>>8)>>outputBit)&1 == 1 {
						C |= 1 << ((7-outputBit)*8 + inputBit)
					}
				}
			}

			// B, D: effect of input hi_byte bits on output
			for inputBit := range 8 {
				result := mulLog(ffe(1<<(inputBit+8)), ffe(logM))
				for outputBit := range 8 {
					if (byte(result)>>outputBit)&1 == 1 {
						B |= 1 << ((7-outputBit)*8 + inputBit)
					}
					if (byte(result>>8)>>outputBit)&1 == 1 {
						D |= 1 << ((7-outputBit)*8 + inputBit)
					}
				}
			}

			gf2p811dMulMatrices16[logM] = [4]uint64{A, B, C, D}
		}
	}
}

func (r *leopardFF16) getShardSlice() [][]byte {
	if s, ok := r.shardSlicePool.Get().([][]byte); ok {
		return s[:r.totalShards]
	}
	return make([][]byte, r.totalShards)
}

func (r *leopardFF16) putShardSlice(s [][]byte) {
	clear(s[:cap(s)])
	r.shardSlicePool.Put(s)
}

func (r *leopardFF16) getWorkSlice(n int) [][]byte {
	if s, ok := r.workSlicePool.Get().([][]byte); ok && cap(s) >= n {
		return s[:n]
	}
	return make([][]byte, n)
}

func (r *leopardFF16) putWorkSlice(s [][]byte) {
	clear(s[:cap(s)])
	r.workSlicePool.Put(s)
}

const kWordMips = 5
const kWords = order / 64
const kBigMips = 6
const kBigWords = (kWords + 63) / 64
const kBiggestMips = 4

// errorBitfield contains progressive errors to help indicate which
// shards need reconstruction.
type errorBitfield struct {
	Words        [kWordMips][kWords]uint64
	BigWords     [kBigMips][kBigWords]uint64
	BiggestWords [kBiggestMips]uint64
	// neededFns holds pre-computed needed-check functions indexed sequentially
	// for each FFT layer, allocated once in prepare to avoid per-call closure allocs.
	// Max 9 entries: mipLevel 16 down to 0 by 2.
	neededFns [9]func(int) bool
}

func (e *errorBitfield) set(i int) {
	e.Words[0][i/64] |= uint64(1) << (i & 63)
}

func (e *errorBitfield) isNeededFn(mipLevel int) func(bit int) bool {
	if mipLevel >= 16 {
		return func(bit int) bool {
			return true
		}
	}
	if mipLevel >= 12 {
		w := e.BiggestWords[mipLevel-12]
		return func(bit int) bool {
			bit /= 4096
			return 0 != (w & (uint64(1) << bit))
		}
	}
	if mipLevel >= 6 {
		w := e.BigWords[mipLevel-6][:]
		return func(bit int) bool {
			bit /= 64
			return 0 != (w[bit/64] & (uint64(1) << (bit & 63)))
		}
	}
	if mipLevel > 0 {
		w := e.Words[mipLevel-1][:]
		return func(bit int) bool {
			return 0 != (w[bit/64] & (uint64(1) << (bit & 63)))
		}
	}
	return nil
}

func (e *errorBitfield) isNeeded(mipLevel int, bit uint) bool {
	if mipLevel >= 16 {
		return true
	}
	if mipLevel >= 12 {
		bit /= 4096
		return 0 != (e.BiggestWords[mipLevel-12] & (uint64(1) << bit))
	}
	if mipLevel >= 6 {
		bit /= 64
		return 0 != (e.BigWords[mipLevel-6][bit/64] & (uint64(1) << (bit % 64)))
	}
	return 0 != (e.Words[mipLevel-1][bit/64] & (uint64(1) << (bit % 64)))
}

var kHiMasks = [5]uint64{
	0xAAAAAAAAAAAAAAAA,
	0xCCCCCCCCCCCCCCCC,
	0xF0F0F0F0F0F0F0F0,
	0xFF00FF00FF00FF00,
	0xFFFF0000FFFF0000,
}

func (e *errorBitfield) prepare(m int) {
	for i := range kWords {
		w_i := e.Words[0][i]
		hi2lo0 := w_i | ((w_i & kHiMasks[0]) >> 1)
		lo2hi0 := (w_i & (kHiMasks[0] >> 1)) << 1
		w_i = hi2lo0 | lo2hi0
		e.Words[0][i] = w_i

		bits := 2
		for j := 1; j < kWordMips; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j]) >> bits)
			lo2hi_j := (w_i & (kHiMasks[j] >> bits)) << bits
			w_i = hi2lo_j | lo2hi_j
			e.Words[j][i] = w_i
			bits <<= 1
		}
	}

	for i := range kBigWords {
		w_i := uint64(0)
		bit := uint64(1)
		src := e.Words[kWordMips-1][i*64 : i*64+64]
		for _, w := range src {
			w_i |= (w | (w >> 32) | (w << 32)) & bit
			bit <<= 1
		}
		e.BigWords[0][i] = w_i

		shift := 1
		for j := 1; j < kBigMips; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> shift)
			lo2hi_j := (w_i & (kHiMasks[j-1] >> shift)) << shift
			w_i = hi2lo_j | lo2hi_j
			e.BigWords[j][i] = w_i
			shift <<= 1
		}
	}

	w_i := uint64(0)
	bit := uint64(1)
	for _, w := range e.BigWords[kBigMips-1][:kBigWords] {
		w_i |= (w | (w >> 32) | (w << 32)) & bit
		bit <<= 1
	}
	e.BiggestWords[0] = w_i

	shift := uint64(1)
	for j := 1; j < kBiggestMips; j++ {
		hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> shift)
		lo2hi_j := (w_i & (kHiMasks[j-1] >> shift)) << shift
		w_i = hi2lo_j | lo2hi_j
		e.BiggestWords[j] = w_i
		shift <<= 1
	}

	// Pre-compute needed-check functions for used mip levels.
	// fftDIT starts at bits.Len32(n)-1 and decrements by 2.
	// We store them sequentially from index 0.
	startLevel := bits.Len32(uint32(m)) - 1
	for i, ml := 0, startLevel; ml >= 0; i, ml = i+1, ml-2 {
		e.neededFns[i] = e.isNeededFn(ml)
	}
}

func (e *errorBitfield) fftDIT(work [][]byte, mtrunc, m int, skewLUT []ffe, o *options) {
	// Decimation in time: Unroll 2 layers at a time
	fnIdx := 0

	dist4 := m
	dist := m >> 2
	for dist != 0 {
		needed := e.neededFns[fnIdx]
		fnIdx++
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			if !needed(r) {
				continue
			}
			iEnd := r + dist
			logM01 := skewLUT[iEnd-1]
			logM02 := skewLUT[iEnd+dist-1]
			logM23 := skewLUT[iEnd+dist*2-1]

			// For each set of dist elements:
			for i := r; i < iEnd; i++ {
				fftDIT4(
					work[i:],
					dist,
					logM01,
					logM23,
					logM02,
					o)
			}
		}
		dist4 = dist
		dist >>= 2
	}

	// If there is one layer left:
	if dist4 == 2 {
		needed := e.neededFns[fnIdx]
		for r := 0; r < mtrunc; r += 2 {
			if !needed(r) {
				continue
			}
			logM := skewLUT[r+1-1]

			if logM == modulus {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT2(work[r], work[r+1], logM, o)
			}
		}
	}
}
