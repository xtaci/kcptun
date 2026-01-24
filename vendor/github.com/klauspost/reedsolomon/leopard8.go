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

func (r *leopardFF8) DecodeIdx(dst [][]byte, expectInput []bool, input [][]byte) error {
	return ErrNotSupported
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

	mtrunc := min(r.dataShards, m)

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
		fwht8(&errLocs, m+r.dataShards)

		for i := range order8 {
			errLocs[i] = ffe8((uint(errLocs[i]) * uint(logWalsh8[i])) % modulus8)
		}

		fwht8(&errLocs, order8)

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
				clear(work[i])
			}
		}
		for i := r.parityShards; i < m; i++ {
			clear(work[i])
		}

		// work <- original data

		for i := 0; i < r.dataShards; i++ {
			if len(sh[i]) != 0 {
				mulgf8(work[m+i], sh[i], errLocs[m+i], &r.o)
			} else {
				clear(work[m+i])
			}
		}
		for i := m + r.dataShards; i < n; i++ {
			clear(work[i])
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
	for i := range mtrunc {
		copy(work[i], data[i])
	}
	for i := mtrunc; i < m; i++ {
		clear(work[i])
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

// addMod returns the sum of a and b modulo modulus. This can return modulus but
// it is expected that callers will convert modulus to 0.
func addMod8(a, b ffe8) ffe8 {
	sum := uint(a) + uint(b)

	// Partial reduction step which allows for modulus to be returned.
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
func fwht8(data *[order8]ffe8, mtrunc int) {
	// Decimation in time: Unroll 2 layers at a time
	dist := 1
	dist4 := 4
	for dist4 <= order8 {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			// For each set of dist elements:
			// Use 16 bit indices to avoid bounds check on [65536]ffe8.
			dist := uint16(dist)
			off := uint16(r)
			for range dist {
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
	for i := range ffe8(modulus8) {
		expLUT8[state] = i
		state <<= 1
		if state >= order8 {
			state ^= polynomial8
		}
	}
	expLUT8[0] = modulus8

	// Conversion to Cantor basis:

	logLUT8[0] = 0
	for i := range bitwidth8 {
		basis := cantorBasis[i]
		width := 1 << i

		for j := range width {
			logLUT8[j+width] = logLUT8[j] ^ basis
		}
	}

	for i := range order8 {
		logLUT8[i] = expLUT8[logLUT8[i]]
	}

	for i := range order8 {
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

	for m := range bitwidth8 - 1 {
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

	for i := range modulus8 {
		fftSkew8[i] = logLUT8[fftSkew8[i]]
	}

	// Precalculate FWHT(Log[i]):

	for i := range order8 {
		logWalsh8[i] = logLUT8[i]
	}
	logWalsh8[0] = 0

	fwht8(logWalsh8, order8)
}

func initMul8LUT() {
	mul8LUTs = &[order8]mul8LUT{}

	// For each log_m multiplicand:
	for log_m := range order8 {
		var tmp [64]ffe8
		for nibble, shift := 0, 0; nibble < 4; {
			nibble_lut := tmp[nibble*16:]

			for xnibble := range 16 {
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
			for i := range 2 {
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
	for i := range kWords8 {
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

	for i := range kWords8 {
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

var gf2p811dMulMatricesLeo8 = [256]uint64{
	0x0102040810204080, 0xaeb88835d2a91d5e, 0xf2106b5d296cc1ca, 0x80d297dc3209edce,
	0x5e29122cc39bf946, 0xca326a1455afa32d, 0xcec3395ae95ce1ba, 0x4655e042717244a8,
	0x2de9eaa5c8de95c2, 0xba7167d17694aafb, 0xa8c82a3f56047a1b, 0xc27624d0ff88ebf1,
	0xfb562191936bc996, 0x1bff07229a97d8bc, 0xf1939e110112a498, 0x969a897cae6a7fb9,
	0xbc01c5dbf239cdbe, 0x98ae65b280e05020, 0xb9f2929d5eeacfa9, 0xbe80349fca67e86c,
	0x205ef327ce2adf09, 0xa9ca2e3746243a9b, 0x6cceace52d21f6af, 0x09464accba07085c,
	0x9b2d90fea89e3572, 0xafba8c3dc2895dde, 0x5ca8e368fbc5dc94, 0x72c2fc811b652c04,
	0xdefb85f0f1921488, 0x941b783896345a6b, 0x04f1534ebcf34297, 0x8896d918982ea512,
	0x6bbc0ae7b9acd16a, 0x97988d74be4a3f39, 0x12b94dee2090d0e0, 0x6abe0eefa98c91ea,
	0x392005416ce32267, 0xe0a926b309fc112a, 0xea6c99339b857c24, 0x6709176daf78db21,
	0x2a9b4ca75c53b207, 0x24afa06972d99d9e, 0x215cf72fde0a9f89, 0x0772a602948d27c5,
	0x9edec7b8044d3765, 0x8994dd10880ee592, 0xc50482d26b05cc34, 0x6588e6299726fef3,
	0x926bda3212993d2e, 0x34971cc36a1768ac, 0xf3126f55394c814a, 0x2e6a1fe9e0a0f090,
	0xac397971eaf7388c, 0x4ae0fdc867a64ee3, 0x90ea2b762ac718fc, 0x8c678a5624dde785,
	0xe32ad3ff21827478, 0xfc24879307e6ee53, 0x8521c09a9edaefd9, 0x78074301891c410a,
	0x539e0baec56fb38d, 0xd98923f2651f334d, 0x0ac5bf8092796d0e, 0x8d658e5e34fda705,
	0x4d925bcaf32b6926, 0x0e34ecce2e8a2f99, 0x05f35746acd30217, 0x262e512d4a87b84c,
	0x99ac61ba90c010a0, 0x174a1aa88c43d2f7, 0x4c905fc2e30b29a6, 0xa08c64fbfc2332c7,
	0xf7e33c1b85bfc3dd, 0xa6fcc6f1788e5582, 0xc7857396535be9e6, 0xdd7870bcd9ec71da,
	0x825366980a57c81c, 0xe6d984b98d51766f, 0xda0ad6be4d61561f, 0x1c8da1200e1aff79,
	0x6f4d59a9055f93fd, 0x1f0e546c26649a2b, 0x79054709993c018a, 0xfd26839b17c6aed3,
	0x2b9948af4c73f287, 0x8a17285ca07080c0, 0xd34c9c72f7665e43, 0x87a031dea684ca0b,
	0xc0f7d594c7d6ce23, 0x43a6b704dda146bf, 0x0bc7bb8882592d8e, 0x23dd066be654ba5b,
	0xbf823097da47a8ec, 0x8ee67b121c83c257, 0x5bda456a6f48fb51, 0xec1c3b391f281b61,
	0x576f58e0799cf11a, 0x511ffaeafd31965f, 0x6179b5672bd5bc64, 0x1afd032a8ab7983c,
	0x5f2b1624d3bbb9c6, 0x648ae2218706be73, 0x3cd35207c0302070, 0xc687779e437ba966,
	0x73c0f8890b456c84, 0x70430dc5233b09d6, 0x660b1365bf589ba1, 0x8423c4928efaaf59,
	0xd6bfcb345bb55c54, 0xa18e60f3ec037247, 0x595bb42e5716de83, 0x54ecadac51e29448,
	0x4757e44a61520428, 0x835162901a77889c, 0x48610c8c5ff86b31, 0x281abde3640d97d5,
	0x9c5f36fc3c1312b7, 0x31644b85c6c46abb, 0xd53c3e7873cb3906, 0xb7c67e537060e030,
	0xbb7363d966b4ea7b, 0x0670a20a84ad6745, 0x30664f8dd6e42a3b, 0x7b84b64da1622458,
	0x45d6150e590c21fa, 0x3ba1f40554bd07b5, 0x5859b02647369e03, 0xfa542599834b8916,
	0xb5478f17483ec5e2, 0x0383f54c287e6552, 0x16481ea09c639277, 0xe228d7f731a234f8,
	0x529c0fa6d54ff30d, 0x7731abc7b7b62e13, 0xf8d5d4ddbb15acc4, 0x0db7198206f44acb,
	0x13bb49e630b09060, 0xc40686da7b258cb4, 0xcb306e1c458fe3ad, 0x607bb16f3bf5fce4,
	0xb4458b1f581e8562, 0xad3b7d79fad7780c, 0xe45875fdb50f53bd, 0x62fa402b03abd936,
	0x0cb51d8a16d40a4b, 0xbd03c1d3e2198d3e, 0x3616ed8752494d7e, 0x4be2f9c077860e63,
	0x3e52a343f86e05a2, 0x7e77e10b0db1264f, 0x63f84423138b99b6, 0xa20d95bfc47d1715,
	0x4f13aa8ecb754cf4, 0xb6c47a5b6040a0b0, 0x15cbebecb41df725, 0xf460c957adc1a68f,
	0xb0b4d851e4edc7f5, 0x25ada46162f9dd1e, 0x8fe47f1a0ca382d7, 0xf562cd5fbde1e60f,
	0x1e0c50643644daab, 0xd7bdcf3c4b951cd4, 0x0f36e8c63eaa6f19, 0xab4bdf737e7a1f49,
	0xd43e3a7063eb7986, 0x197ef666a2c9fd6e, 0x496308844fd82bb1, 0x86a235d6b6a48a8b,
	0x6e4f5da1157fd37d, 0xb1b6dc59f4cd8775, 0x8b152c54b050c040, 0x7df4144725cf431d,
	0x75b05a838fe80bc1, 0x40254248f5df23ed, 0x1d8fa5281e3abff9, 0xc1f5d19cd7f68ea3,
	0xed1e3f310f085be1, 0xf9d7d0d5ab35ec44, 0xa30f91b7d45d5795, 0xe1ab22bb19dc51aa,
	0x44d41106492c617a, 0x95197c3086141aeb, 0xaa49db7b6e5a5fc9, 0x7a86b245b14264d8,
	0xeb6e9d3b8ba53ca4, 0xc9b19f587dd1c67f, 0xd88b27fa753f73cd, 0xa47d37b540d07050,
	0x7f75e5031d9166cf, 0xcd40cc16c12284e8, 0x501dfee2ed11d6df, 0xcfc13d52f97ca13a,
	0xe8ed6877a3db59f6, 0xdff981f8e1b25408, 0x3aa3f00d449d4735, 0xf6e13813959f835d,
	0x08444ec4aa2748dc, 0x359518cb7a37282c, 0x5daae760ebe59c14, 0xdc7a74b4c9cc315a,
	0x2cebeeadd8fed542, 0x14c9efe4a43db7a5, 0x5ad841627f68bbd1, 0x42a4b30ccd81063f,
	0xa57f33bd50f030d0, 0xd1cd6d36cf387b91, 0x3f50a74be84e4522, 0xd0cf693edf183b11,
	0x91e82f7e3ae7587c, 0x22df0263f674fadb, 0x113ab8a208eeb5b2, 0x7cf6104f35ef039d,
	0xdb08d2b65d41169f, 0xb2352915dcb3e227, 0x9d5d32f42c335237, 0x9fdcc3b0146d77e5,
	0x272c55255aa7f8cc, 0x3714e98f42690dfe, 0xe55a71f5a52f133d, 0xcc42c81ed102c468,
	0xfea576d73fb8cb81, 0x3dd1560fd01060f0, 0x683fffab91d2b438, 0x81d093d42229ad4e,
	0xf0919a191132e418, 0x382201497cc362e7, 0x4e11ae86db550c74, 0x187cf26eb2e9bdee,
	0xe7db80b19d7136ef, 0x74b25e8b9fc84b41, 0xee9dca7d27763eb3, 0xef9fce7537567e33,
	0x41274640e5ff636d, 0xb3372d1dcc93a2a7, 0x33e5bac1fe9a4f69, 0x6dcca8ed3d01b62f,
	0xa7fec2f968ae1502, 0x693dfba381f2f4b8, 0x2f681be1f080b010, 0x0281f144385e25d2,
	0xb8f096954eca8f29, 0x1038bcaa18cef532, 0xd24e987ae7461ec3, 0x2918b9eb742dd755,
	0x32e7bec9eeba0fe9, 0xc37420d8efa8ab71, 0x55eea9a441c2d4c8, 0xe9ef6c7fb3fb1976,
	0x714109cd331b4956, 0xc8b39b506df186ff, 0x7633afcfa7966e93, 0x566d5ce869bcb19a,
	0xffa772df2f988b01, 0x9369de3a02b97dae, 0x9a2f94f6b8be75f2, 0x0102040810204080,
}
