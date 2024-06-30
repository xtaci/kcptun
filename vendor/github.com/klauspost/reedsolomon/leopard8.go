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
	"encoding/binary"
	"io"
	"math/bits"
	"sync"
)

// leopardFF8 is like reedSolomon but for the 8-bit "leopard" implementation.
type leopardFF8 struct {
	dataShards   int // Number of data shards, should not be modified.
	parityShards int // Number of parity shards, should not be modified.
	totalShards  int // Total number of shards. Calculated, and should not be modified.

	workPool    sync.Pool
	inversion   map[[inversion8Bytes]byte]leopardGF8cache
	inversionMu sync.Mutex

	o options
}

const inversion8Bytes = 256 / 8

type leopardGF8cache struct {
	errorLocs [256]ffe8
	bits      *errorBitfield8
}

// newFF8 is like New, but for the 8-bit "leopard" implementation.
func newFF8(dataShards, parityShards int, opt options) (*leopardFF8, error) {
	initConstants8()

	if dataShards <= 0 || parityShards <= 0 {
		return nil, ErrInvShardNum
	}

	if dataShards+parityShards > 65536 {
		return nil, ErrMaxShardNum
	}

	r := &leopardFF8{
		dataShards:   dataShards,
		parityShards: parityShards,
		totalShards:  dataShards + parityShards,
		o:            opt,
	}
	if opt.inversionCache && (r.totalShards <= 64 || opt.forcedInversionCache) {
		// Inversion cache is relatively ineffective for big shard counts and takes up potentially lots of memory
		// r.totalShards is not covering the space, but an estimate.
		r.inversion = make(map[[inversion8Bytes]byte]leopardGF8cache, r.totalShards)
	}
	return r, nil
}

var _ = Extensions(&leopardFF8{})

func (r *leopardFF8) ShardSizeMultiple() int {
	return 64
}

func (r *leopardFF8) DataShards() int {
	return r.dataShards
}

func (r *leopardFF8) ParityShards() int {
	return r.parityShards
}

func (r *leopardFF8) TotalShards() int {
	return r.totalShards
}

func (r *leopardFF8) AllocAligned(each int) [][]byte {
	return AllocAligned(r.totalShards, each)
}

type ffe8 uint8

const (
	bitwidth8   = 8
	order8      = 1 << bitwidth8
	modulus8    = order8 - 1
	polynomial8 = 0x11D

	// Encode in blocks of this size.
	workSize8 = 32 << 10
)

var (
	fftSkew8  *[modulus8]ffe8
	logWalsh8 *[order8]ffe8
)

// Logarithm Tables
var (
	logLUT8 *[order8]ffe8
	expLUT8 *[order8]ffe8
)

// Stores the partial products of x * y at offset x + y * 256
// Repeated accesses from the same y value are faster
var mul8LUTs *[order8]mul8LUT

type mul8LUT struct {
	Value [256]ffe8
}

// Stores lookup for avx2
var multiply256LUT8 *[order8][2 * 16]byte

func (r *leopardFF8) Encode(shards [][]byte) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, false); err != nil {
		return err
	}
	return r.encode(shards)
}

func (r *leopardFF8) encode(shards [][]byte) error {
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	m := ceilPow2(r.parityShards)
	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	} else {
		work = AllocAligned(m*2, workSize8)
	}
	if cap(work) >= m*2 {
		work = work[:m*2]
		for i := range work {
			if i >= r.parityShards {
				if cap(work[i]) < workSize8 {
					work[i] = AllocAligned(1, workSize8)[0]
				} else {
					work[i] = work[i][:workSize8]
				}
			}
		}
	} else {
		work = AllocAligned(m*2, workSize8)
	}

	defer r.workPool.Put(work)

	mtrunc := m
	if r.dataShards < mtrunc {
		mtrunc = r.dataShards
	}

	skewLUT := fftSkew8[m-1:]

	// Split large shards.
	// More likely on lower shard count.
	off := 0
	sh := make([][]byte, len(shards))

	// work slice we can modify
	wMod := make([][]byte, len(work))
	copy(wMod, work)
	for off < shardSize {
		work := wMod
		sh := sh
		end := off + workSize8
		if end > shardSize {
			end = shardSize
			sz := shardSize - off
			for i := range work {
				// Last iteration only...
				work[i] = work[i][:sz]
			}
		}
		for i := range shards {
			sh[i] = shards[i][off:end]
		}

		// Replace work slices, so we write directly to output.
		// Note that work has parity *before* data shards.
		res := shards[r.dataShards:r.totalShards]
		for i := range res {
			work[i] = res[i][off:end]
		}

		ifftDITEncoder8(
			sh[:r.dataShards],
			mtrunc,
			work,
			nil, // No xor output
			m,
			skewLUT,
			&r.o,
		)

		lastCount := r.dataShards % m
		skewLUT2 := skewLUT
		if m >= r.dataShards {
			goto skip_body
		}

		// For sets of m data pieces:
		for i := m; i+m <= r.dataShards; i += m {
			sh = sh[m:]
			skewLUT2 = skewLUT2[m:]

			// work <- work xor IFFT(data + i, m, m + i)

			ifftDITEncoder8(
				sh, // data source
				m,
				work[m:], // temporary workspace
				work,     // xor destination
				m,
				skewLUT2,
				&r.o,
			)
		}

		// Handle final partial set of m pieces:
		if lastCount != 0 {
			sh = sh[m:]
			skewLUT2 = skewLUT2[m:]

			// work <- work xor IFFT(data + i, m, m + i)

			ifftDITEncoder8(
				sh, // data source
				lastCount,
				work[m:], // temporary workspace
				work,     // xor destination
				m,
				skewLUT2,
				&r.o,
			)
		}

	skip_body:
		// work <- FFT(work, m, 0)
		fftDIT8(work, r.parityShards, m, fftSkew8[:], &r.o)
		off += workSize8
	}

	return nil
}

func (r *leopardFF8) EncodeIdx(dataShard []byte, idx int, parity [][]byte) error {
	return ErrNotSupported
}

func (r *leopardFF8) Join(dst io.Writer, shards [][]byte, outSize int) error {
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

func (r *leopardFF8) Update(shards [][]byte, newDatashards [][]byte) error {
	return ErrNotSupported
}

func (r *leopardFF8) Split(data []byte) ([][]byte, error) {
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

func (r *leopardFF8) ReconstructSome(shards [][]byte, required []bool) error {
	if len(required) == r.totalShards {
		return r.reconstruct(shards, true)
	}
	return r.reconstruct(shards, false)
}

func (r *leopardFF8) Reconstruct(shards [][]byte) error {
	return r.reconstruct(shards, true)
}

func (r *leopardFF8) ReconstructData(shards [][]byte) error {
	return r.reconstruct(shards, false)
}

func (r *leopardFF8) Verify(shards [][]byte) (bool, error) {
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

func (r *leopardFF8) reconstruct(shards [][]byte, recoverAll bool) error {
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

	// Check if we have enough to reconstruct.
	if numberPresent < r.dataShards {
		return ErrTooFewShards
	}

	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	// Use only if we are missing less than 1/4 parity,
	// And we are restoring a significant amount of data.
	useBits := r.totalShards-numberPresent <= r.parityShards/4 && shardSize*r.totalShards >= 64<<10

	m := ceilPow2(r.parityShards)
	n := ceilPow2(m + r.dataShards)

	const LEO_ERROR_BITFIELD_OPT = true

	// Fill in error locations.
	var errorBits errorBitfield8
	var errLocs [order8]ffe8
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
		}
	}

	var gotInversion bool
	if LEO_ERROR_BITFIELD_OPT && r.inversion != nil {
		cacheID := errorBits.cacheID()
		r.inversionMu.Lock()
		if inv, ok := r.inversion[cacheID]; ok {
			r.inversionMu.Unlock()
			errLocs = inv.errorLocs
			if inv.bits != nil && useBits {
				errorBits = *inv.bits
				useBits = true
			} else {
				useBits = false
			}
			gotInversion = true
		} else {
			r.inversionMu.Unlock()
		}
	}

	if !gotInversion {
		// No inversion...
		if LEO_ERROR_BITFIELD_OPT && useBits {
			errorBits.prepare()
		}

		// Evaluate error locator polynomial8
		fwht8(&errLocs, order8, m+r.dataShards)

		for i := 0; i < order8; i++ {
			errLocs[i] = ffe8((uint(errLocs[i]) * uint(logWalsh8[i])) % modulus8)
		}

		fwht8(&errLocs, order8, order8)

		if r.inversion != nil {
			c := leopardGF8cache{
				errorLocs: errLocs,
			}
			if useBits {
				// Heap alloc
				var x errorBitfield8
				x = errorBits
				c.bits = &x
			}
			r.inversionMu.Lock()
			r.inversion[errorBits.cacheID()] = c
			r.inversionMu.Unlock()
		}
	}

	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	if cap(work) >= n {
		work = work[:n]
		for i := range work {
			if cap(work[i]) < workSize8 {
				work[i] = make([]byte, workSize8)
			} else {
				work[i] = work[i][:workSize8]
			}
		}

	} else {
		work = make([][]byte, n)
		all := make([]byte, n*workSize8)
		for i := range work {
			work[i] = all[i*workSize8 : i*workSize8+workSize8]
		}
	}
	defer r.workPool.Put(work)

	// work <- recovery data

	// Split large shards.
	// More likely on lower shard count.
	sh := make([][]byte, len(shards))
	// Copy...
	copy(sh, shards)

	// Add output
	for i, sh := range shards {
		if !recoverAll && i >= r.dataShards {
			continue
		}
		if len(sh) == 0 {
			if cap(sh) >= shardSize {
				shards[i] = sh[:shardSize]
			} else {
				shards[i] = make([]byte, shardSize)
			}
		}
	}

	off := 0
	for off < shardSize {
		endSlice := off + workSize8
		if endSlice > shardSize {
			endSlice = shardSize
			sz := shardSize - off
			// Last iteration only
			for i := range work {
				work[i] = work[i][:sz]
			}
		}
		for i := range shards {
			if len(sh[i]) != 0 {
				sh[i] = shards[i][off:endSlice]
			}
		}
		for i := 0; i < r.parityShards; i++ {
			if len(sh[i+r.dataShards]) != 0 {
				mulgf8(work[i], sh[i+r.dataShards], errLocs[i], &r.o)
			} else {
				memclr(work[i])
			}
		}
		for i := r.parityShards; i < m; i++ {
			memclr(work[i])
		}

		// work <- original data

		for i := 0; i < r.dataShards; i++ {
			if len(sh[i]) != 0 {
				mulgf8(work[m+i], sh[i], errLocs[m+i], &r.o)
			} else {
				memclr(work[m+i])
			}
		}
		for i := m + r.dataShards; i < n; i++ {
			memclr(work[i])
		}

		// work <- IFFT(work, n, 0)

		ifftDITDecoder8(
			m+r.dataShards,
			work,
			n,
			fftSkew8[:],
			&r.o,
		)

		// work <- FormalDerivative(work, n)

		for i := 1; i < n; i++ {
			width := ((i ^ (i - 1)) + 1) >> 1
			slicesXor(work[i-width:i], work[i:i+width], &r.o)
		}

		// work <- FFT(work, n, 0) truncated to m + dataShards

		outputCount := m + r.dataShards

		if LEO_ERROR_BITFIELD_OPT && useBits {
			errorBits.fftDIT8(work, outputCount, n, fftSkew8[:], &r.o)
		} else {
			fftDIT8(work, outputCount, n, fftSkew8[:], &r.o)
		}

		// Reveal erasures
		//
		//  Original = -ErrLocator * FFT( Derivative( IFFT( ErrLocator * ReceivedData ) ) )
		//  mul_mem(x, y, log_m, ) equals x[] = y[] * log_m
		//
		// mem layout: [Recovery Data (Power of Two = M)] [Original Data (K)] [Zero Padding out to N]
		end := r.dataShards
		if recoverAll {
			end = r.totalShards
		}
		// Restore
		for i := 0; i < end; i++ {
			if len(sh[i]) != 0 {
				continue
			}

			if i >= r.dataShards {
				// Parity shard.
				mulgf8(shards[i][off:endSlice], work[i-r.dataShards], modulus8-errLocs[i-r.dataShards], &r.o)
			} else {
				// Data shard.
				mulgf8(shards[i][off:endSlice], work[i+m], modulus8-errLocs[i+m], &r.o)
			}
		}
		off += workSize8
	}
	return nil
}

// Basic no-frills version for decoder
func ifftDITDecoder8(mtrunc int, work [][]byte, m int, skewLUT []ffe8, o *options) {
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
				ifftDIT48(work[i:], dist, log_m01, log_m23, log_m02, o)
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

		if log_m == modulus8 {
			slicesXor(work[dist:2*dist], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT28(
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
func fftDIT8(work [][]byte, mtrunc, m int, skewLUT []ffe8, o *options) {
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
				fftDIT48(
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

			if log_m == modulus8 {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT28(work[r], work[r+1], log_m, o)
			}
		}
	}
}

// 4-way butterfly
func fftDIT4Ref8(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	// First layer:
	if log_m02 == modulus8 {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		fftDIT28(work[0], work[dist*2], log_m02, o)
		fftDIT28(work[dist], work[dist*3], log_m02, o)
	}

	// Second layer:
	if log_m01 == modulus8 {
		sliceXor(work[0], work[dist], o)
	} else {
		fftDIT28(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus8 {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		fftDIT28(work[dist*2], work[dist*3], log_m23, o)
	}
}

// Unrolled IFFT for encoder
func ifftDITEncoder8(data [][]byte, mtrunc int, work [][]byte, xorRes [][]byte, m int, skewLUT []ffe8, o *options) {
	// I tried rolling the memcpy/memset into the first layer of the FFT and
	// found that it only yields a 4% performance improvement, which is not
	// worth the extra complexity.
	for i := 0; i < mtrunc; i++ {
		copy(work[i], data[i])
	}
	for i := mtrunc; i < m; i++ {
		memclr(work[i])
	}

	// Decimation in time: Unroll 2 layers at a time
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			iend := r + dist
			log_m01 := skewLUT[iend]
			log_m02 := skewLUT[iend+dist]
			log_m23 := skewLUT[iend+dist*2]

			// For each set of dist elements:
			for i := r; i < iend; i++ {
				ifftDIT48(
					work[i:],
					dist,
					log_m01,
					log_m23,
					log_m02,
					o,
				)
			}
		}

		dist = dist4
		dist4 <<= 2
		// I tried alternating sweeps left->right and right->left to reduce cache misses.
		// It provides about 1% performance boost when done for both FFT and IFFT, so it
		// does not seem to be worth the extra complexity.
	}

	// If there is one layer left:
	if dist < m {
		// Assuming that dist = m / 2
		if dist*2 != m {
			panic("internal error")
		}

		logm := skewLUT[dist]

		if logm == modulus8 {
			slicesXor(work[dist:dist*2], work[:dist], o)
		} else {
			for i := 0; i < dist; i++ {
				ifftDIT28(work[i], work[i+dist], logm, o)
			}
		}
	}

	// I tried unrolling this but it does not provide more than 5% performance
	// improvement for 16-bit finite fields, so it's not worth the complexity.
	if xorRes != nil {
		slicesXor(xorRes[:m], work[:m], o)
	}
}

func ifftDIT4Ref8(work [][]byte, dist int, log_m01, log_m23, log_m02 ffe8, o *options) {
	// First layer:
	if log_m01 == modulus8 {
		sliceXor(work[0], work[dist], o)
	} else {
		ifftDIT28(work[0], work[dist], log_m01, o)
	}

	if log_m23 == modulus8 {
		sliceXor(work[dist*2], work[dist*3], o)
	} else {
		ifftDIT28(work[dist*2], work[dist*3], log_m23, o)
	}

	// Second layer:
	if log_m02 == modulus8 {
		sliceXor(work[0], work[dist*2], o)
		sliceXor(work[dist], work[dist*3], o)
	} else {
		ifftDIT28(work[0], work[dist*2], log_m02, o)
		ifftDIT28(work[dist], work[dist*3], log_m02, o)
	}
}

// Reference version of muladd: x[] ^= y[] * log_m
func refMulAdd8(x, y []byte, log_m ffe8) {
	lut := &mul8LUTs[log_m]

	for len(x) >= 64 {
		// Assert sizes for no bounds checks in loop
		src := y[:64]
		dst := x[:len(src)] // Needed, but not checked...
		for i, y1 := range src {
			dst[i] ^= byte(lut.Value[y1])
		}
		x = x[64:]
		y = y[64:]
	}
}

// Reference version of mul: x[] = y[] * log_m
func refMul8(x, y []byte, log_m ffe8) {
	lut := &mul8LUTs[log_m]

	for off := 0; off < len(x); off += 64 {
		src := y[off : off+64]
		for i, y1 := range src {
			x[off+i] = byte(lut.Value[y1])
		}
	}
}

// Returns a * Log(b)
func mulLog8(a, log_b ffe8) ffe8 {
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
	return expLUT8[addMod8(logLUT8[a], log_b)]
}

// z = x + y (mod kModulus)
func addMod8(a, b ffe8) ffe8 {
	sum := uint(a) + uint(b)

	// Partial reduction step, allowing for kModulus to be returned
	return ffe8(sum + sum>>bitwidth8)
}

// z = x - y (mod kModulus)
func subMod8(a, b ffe8) ffe8 {
	dif := uint(a) - uint(b)

	// Partial reduction step, allowing for kModulus to be returned
	return ffe8(dif + dif>>bitwidth8)
}

// Decimation in time (DIT) Fast Walsh-Hadamard Transform
// Unrolls pairs of layers to perform cross-layer operations in registers
// mtrunc: Number of elements that are non-zero at the front of data
func fwht8(data *[order8]ffe8, m, mtrunc int) {
	// Decimation in time: Unroll 2 layers at a time
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			// For each set of dist elements:
			// Use 16 bit indices to avoid bounds check on [65536]ffe8.
			dist := uint16(dist)
			off := uint16(r)
			for i := uint16(0); i < dist; i++ {
				// fwht48(data[i:], dist) inlined...
				// Reading values appear faster than updating pointers.
				// Casting to uint is not faster.
				t0 := data[off]
				t1 := data[off+dist]
				t2 := data[off+dist*2]
				t3 := data[off+dist*3]

				t0, t1 = fwht2alt8(t0, t1)
				t2, t3 = fwht2alt8(t2, t3)
				t0, t2 = fwht2alt8(t0, t2)
				t1, t3 = fwht2alt8(t1, t3)

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

	// If there is one layer left:
	if dist < m {
		dist := uint16(dist)
		for i := uint16(0); i < dist; i++ {
			fwht28(&data[i], &data[i+dist])
		}
	}
}

func fwht48(data []ffe8, s int) {
	s2 := s << 1

	t0 := &data[0]
	t1 := &data[s]
	t2 := &data[s2]
	t3 := &data[s2+s]

	fwht28(t0, t1)
	fwht28(t2, t3)
	fwht28(t0, t2)
	fwht28(t1, t3)
}

// {a, b} = {a + b, a - b} (Mod Q)
func fwht28(a, b *ffe8) {
	sum := addMod8(*a, *b)
	dif := subMod8(*a, *b)
	*a = sum
	*b = dif
}

// fwht2alt8  is as fwht28, but returns result.
func fwht2alt8(a, b ffe8) (ffe8, ffe8) {
	return addMod8(a, b), subMod8(a, b)
}

var initOnce8 sync.Once

func initConstants8() {
	initOnce8.Do(func() {
		initLUTs8()
		initFFTSkew8()
		initMul8LUT()
	})
}

// Initialize logLUT8, expLUT8.
func initLUTs8() {
	cantorBasis := [bitwidth8]ffe8{
		1, 214, 152, 146, 86, 200, 88, 230,
	}

	expLUT8 = &[order8]ffe8{}
	logLUT8 = &[order8]ffe8{}

	// LFSR table generation:
	state := 1
	for i := ffe8(0); i < modulus8; i++ {
		expLUT8[state] = i
		state <<= 1
		if state >= order8 {
			state ^= polynomial8
		}
	}
	expLUT8[0] = modulus8

	// Conversion to Cantor basis:

	logLUT8[0] = 0
	for i := 0; i < bitwidth8; i++ {
		basis := cantorBasis[i]
		width := 1 << i

		for j := 0; j < width; j++ {
			logLUT8[j+width] = logLUT8[j] ^ basis
		}
	}

	for i := 0; i < order8; i++ {
		logLUT8[i] = expLUT8[logLUT8[i]]
	}

	for i := 0; i < order8; i++ {
		expLUT8[logLUT8[i]] = ffe8(i)
	}

	expLUT8[modulus8] = expLUT8[0]
}

// Initialize fftSkew8.
func initFFTSkew8() {
	var temp [bitwidth8 - 1]ffe8

	// Generate FFT skew vector {1}:

	for i := 1; i < bitwidth8; i++ {
		temp[i-1] = ffe8(1 << i)
	}

	fftSkew8 = &[modulus8]ffe8{}
	logWalsh8 = &[order8]ffe8{}

	for m := 0; m < bitwidth8-1; m++ {
		step := 1 << (m + 1)

		fftSkew8[1<<m-1] = 0

		for i := m; i < bitwidth8-1; i++ {
			s := 1 << (i + 1)

			for j := 1<<m - 1; j < s; j += step {
				fftSkew8[j+s] = fftSkew8[j] ^ temp[i]
			}
		}

		temp[m] = modulus8 - logLUT8[mulLog8(temp[m], logLUT8[temp[m]^1])]

		for i := m + 1; i < bitwidth8-1; i++ {
			sum := addMod8(logLUT8[temp[i]^1], temp[m])
			temp[i] = mulLog8(temp[i], sum)
		}
	}

	for i := 0; i < modulus8; i++ {
		fftSkew8[i] = logLUT8[fftSkew8[i]]
	}

	// Precalculate FWHT(Log[i]):

	for i := 0; i < order8; i++ {
		logWalsh8[i] = logLUT8[i]
	}
	logWalsh8[0] = 0

	fwht8(logWalsh8, order8, order8)
}

func initMul8LUT() {
	mul8LUTs = &[order8]mul8LUT{}

	// For each log_m multiplicand:
	for log_m := 0; log_m < order8; log_m++ {
		var tmp [64]ffe8
		for nibble, shift := 0, 0; nibble < 4; {
			nibble_lut := tmp[nibble*16:]

			for xnibble := 0; xnibble < 16; xnibble++ {
				prod := mulLog8(ffe8(xnibble<<shift), ffe8(log_m))
				nibble_lut[xnibble] = prod
			}
			nibble++
			shift += 4
		}
		lut := &mul8LUTs[log_m]
		for i := range lut.Value[:] {
			lut.Value[i] = tmp[i&15] ^ tmp[((i>>4)+16)]
		}
	}
	// Always initialize assembly tables.
	// Not as big resource hog as gf16.
	if true {
		multiply256LUT8 = &[order8][16 * 2]byte{}

		for logM := range multiply256LUT8[:] {
			// For each 4 bits of the finite field width in bits:
			shift := 0
			for i := 0; i < 2; i++ {
				// Construct 16 entry LUT for PSHUFB
				prod := multiply256LUT8[logM][i*16 : i*16+16]
				for x := range prod[:] {
					prod[x] = byte(mulLog8(ffe8(x<<shift), ffe8(logM)))
				}
				shift += 4
			}
		}
	}
}

const kWords8 = order8 / 64

// errorBitfield contains progressive errors to help indicate which
// shards need reconstruction.
type errorBitfield8 struct {
	Words [7][kWords8]uint64
}

func (e *errorBitfield8) set(i int) {
	e.Words[0][(i/64)&3] |= uint64(1) << (i & 63)
}

func (e *errorBitfield8) cacheID() [inversion8Bytes]byte {
	var res [inversion8Bytes]byte
	binary.LittleEndian.PutUint64(res[0:8], e.Words[0][0])
	binary.LittleEndian.PutUint64(res[8:16], e.Words[0][1])
	binary.LittleEndian.PutUint64(res[16:24], e.Words[0][2])
	binary.LittleEndian.PutUint64(res[24:32], e.Words[0][3])
	return res
}

func (e *errorBitfield8) isNeeded(mipLevel, bit int) bool {
	if mipLevel >= 8 || mipLevel <= 0 {
		return true
	}
	return 0 != (e.Words[mipLevel-1][bit/64] & (uint64(1) << (bit & 63)))
}

func (e *errorBitfield8) prepare() {
	// First mip level is for final layer of FFT: pairs of data
	for i := 0; i < kWords8; i++ {
		w_i := e.Words[0][i]
		hi2lo0 := w_i | ((w_i & kHiMasks[0]) >> 1)
		lo2hi0 := (w_i & (kHiMasks[0] >> 1)) << 1
		w_i = hi2lo0 | lo2hi0
		e.Words[0][i] = w_i

		bits := 2
		for j := 1; j < 5; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j]) >> bits)
			lo2hi_j := (w_i & (kHiMasks[j] >> bits)) << bits
			w_i = hi2lo_j | lo2hi_j
			e.Words[j][i] = w_i
			bits <<= 1
		}
	}

	for i := 0; i < kWords8; i++ {
		w := e.Words[4][i]
		w |= w >> 32
		w |= w << 32
		e.Words[5][i] = w
	}

	for i := 0; i < kWords8; i += 2 {
		t := e.Words[5][i] | e.Words[5][i+1]
		e.Words[6][i] = t
		e.Words[6][i+1] = t
	}
}

func (e *errorBitfield8) fftDIT8(work [][]byte, mtrunc, m int, skewLUT []ffe8, o *options) {
	// Decimation in time: Unroll 2 layers at a time
	mipLevel := bits.Len32(uint32(m)) - 1

	dist4 := m
	dist := m >> 2
	for dist != 0 {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			if !e.isNeeded(mipLevel, r) {
				continue
			}
			iEnd := r + dist
			logM01 := skewLUT[iEnd-1]
			logM02 := skewLUT[iEnd+dist-1]
			logM23 := skewLUT[iEnd+dist*2-1]

			// For each set of dist elements:
			for i := r; i < iEnd; i++ {
				fftDIT48(
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
		mipLevel -= 2
	}

	// If there is one layer left:
	if dist4 == 2 {
		for r := 0; r < mtrunc; r += 2 {
			if !e.isNeeded(mipLevel, r) {
				continue
			}
			logM := skewLUT[r+1-1]

			if logM == modulus8 {
				sliceXor(work[r], work[r+1], o)
			} else {
				fftDIT28(work[r], work[r+1], logM, o)
			}
		}
	}
}
