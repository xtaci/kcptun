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

// leopardFF16 is like reedSolomon but for more than 256 total shards.
type leopardFF16 struct {
	dataShards   int // Number of data shards, should not be modified.
	parityShards int // Number of parity shards, should not be modified.
	totalShards  int // Total number of shards. Calculated, and should not be modified.

	workPool sync.Pool

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

	r := &leopardFF16{
		dataShards:   dataShards,
		parityShards: parityShards,
		totalShards:  dataShards + parityShards,
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

func (r *leopardFF16) Encode(shards [][]byte) error {
	if len(shards) != r.totalShards {
		return ErrTooFewShards
	}

	if err := checkShards(shards, false); err != nil {
		return err
	}
	return r.encode(shards)
}

func (r *leopardFF16) encode(shards [][]byte) error {
	shardSize := shardSize(shards)
	if shardSize%64 != 0 {
		return ErrInvalidShardSize
	}

	m := ceilPow2(r.parityShards)
	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	if cap(work) >= m*2 {
		work = work[:m*2]
	} else {
		work = AllocAligned(m*2, shardSize)
	}
	for i := range work {
		if cap(work[i]) < shardSize {
			work[i] = AllocAligned(1, shardSize)[0]
		} else {
			work[i] = work[i][:shardSize]
		}
	}
	defer r.workPool.Put(work)

	mtrunc := m
	if r.dataShards < mtrunc {
		mtrunc = r.dataShards
	}

	skewLUT := fftSkew[m-1:]

	sh := shards
	ifftDITEncoder(
		sh[:r.dataShards],
		mtrunc,
		work,
		nil, // No xor output
		m,
		skewLUT,
		&r.o,
	)

	lastCount := r.dataShards % m
	if m >= r.dataShards {
		goto skip_body
	}

	// For sets of m data pieces:
	for i := m; i+m <= r.dataShards; i += m {
		sh = sh[m:]
		skewLUT = skewLUT[m:]

		// work <- work xor IFFT(data + i, m, m + i)

		ifftDITEncoder(
			sh, // data source
			m,
			work[m:], // temporary workspace
			work,     // xor destination
			m,
			skewLUT,
			&r.o,
		)
	}

	// Handle final partial set of m pieces:
	if lastCount != 0 {
		sh = sh[m:]
		skewLUT = skewLUT[m:]

		// work <- work xor IFFT(data + i, m, m + i)

		ifftDITEncoder(
			sh, // data source
			lastCount,
			work[m:], // temporary workspace
			work,     // xor destination
			m,
			skewLUT,
			&r.o,
		)
	}

skip_body:
	// work <- FFT(work, m, 0)
	fftDIT(work, r.parityShards, m, fftSkew[:], &r.o)

	for i, w := range work[:r.parityShards] {
		sh := shards[i+r.dataShards]
		if cap(sh) >= shardSize {
			sh = append(sh[:0], w...)
		} else {
			sh = w
		}
		shards[i+r.dataShards] = sh
	}

	return nil
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
	} else {
		zero := data[dataLen : r.totalShards*perShard]
		for i := range zero {
			zero[i] = 0
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

	// Use only if we are missing less than 1/4 parity.
	useBits := r.totalShards-numberPresent <= r.parityShards/4

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

	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.prepare()
	}

	// Evaluate error locator polynomial
	fwht(&errLocs, order, m+r.dataShards)

	for i := 0; i < order; i++ {
		errLocs[i] = ffe((uint(errLocs[i]) * uint(logWalsh[i])) % modulus)
	}

	fwht(&errLocs, order, order)

	var work [][]byte
	if w, ok := r.workPool.Get().([][]byte); ok {
		work = w
	}
	if cap(work) >= n {
		work = work[:n]
	} else {
		work = make([][]byte, n)
	}
	for i := range work {
		if cap(work[i]) < shardSize {
			work[i] = make([]byte, shardSize)
		} else {
			work[i] = work[i][:shardSize]
		}
	}
	defer r.workPool.Put(work)

	// work <- recovery data

	for i := 0; i < r.parityShards; i++ {
		if len(shards[i+r.dataShards]) != 0 {
			mulgf16(work[i], shards[i+r.dataShards], errLocs[i], &r.o)
		} else {
			memclr(work[i])
		}
	}
	for i := r.parityShards; i < m; i++ {
		memclr(work[i])
	}

	// work <- original data

	for i := 0; i < r.dataShards; i++ {
		if len(shards[i]) != 0 {
			mulgf16(work[m+i], shards[i], errLocs[m+i], &r.o)
		} else {
			memclr(work[m+i])
		}
	}
	for i := m + r.dataShards; i < n; i++ {
		memclr(work[i])
	}

	// work <- IFFT(work, n, 0)

	ifftDITDecoder(
		m+r.dataShards,
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

	outputCount := m + r.dataShards

	if LEO_ERROR_BITFIELD_OPT && useBits {
		errorBits.fftDIT(work, outputCount, n, fftSkew[:], &r.o)
	} else {
		fftDIT(work, outputCount, n, fftSkew[:], &r.o)
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
	for i := 0; i < end; i++ {
		if len(shards[i]) != 0 {
			continue
		}
		if cap(shards[i]) >= shardSize {
			shards[i] = shards[i][:shardSize]
		} else {
			shards[i] = make([]byte, shardSize)
		}
		if i >= r.dataShards {
			// Parity shard.
			mulgf16(shards[i], work[i-r.dataShards], modulus-errLocs[i-r.dataShards], &r.o)
		} else {
			// Data shard.
			mulgf16(shards[i], work[i+m], modulus-errLocs[i+m], &r.o)
		}
	}
	return nil
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
	// I tried rolling the memcpy/memset into the first layer of the FFT and
	// found that it only yields a 4% performance improvement, which is not
	// worth the extra complexity.
	for i := 0; i < mtrunc; i++ {
		copy(work[i], data[i])
	}
	for i := mtrunc; i < m; i++ {
		memclr(work[i])
	}

	// I tried splitting up the first few layers into L3-cache sized blocks but
	// found that it only provides about 5% performance boost, which is not
	// worth the extra complexity.

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
				ifftDIT4(
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

func memclr(s []byte) {
	for i := range s {
		s[i] = 0
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

// z = x + y (mod kModulus)
func addMod(a, b ffe) ffe {
	sum := uint(a) + uint(b)

	// Partial reduction step, allowing for kModulus to be returned
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
func fwht(data *[order]ffe, m, mtrunc int) {
	// Decimation in time: Unroll 2 layers at a time
	dist := 1
	dist4 := 4
	for dist4 <= m {
		// For each set of dist*4 elements:
		for r := 0; r < mtrunc; r += dist4 {
			// For each set of dist elements:
			// Use 16 bit indices to avoid bounds check on [65536]ffe.
			dist := uint16(dist)
			off := uint16(r)
			for i := uint16(0); i < dist; i++ {
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

	// If there is one layer left:
	if dist < m {
		dist := uint16(dist)
		for i := uint16(0); i < dist; i++ {
			fwht2(&data[i], &data[i+dist])
		}
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
	return addMod(a, b), subMod(a, b)
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
	for i := ffe(0); i < modulus; i++ {
		expLUT[state] = i
		state <<= 1
		if state >= order {
			state ^= polynomial
		}
	}
	expLUT[0] = modulus

	// Conversion to Cantor basis:

	logLUT[0] = 0
	for i := 0; i < bitwidth; i++ {
		basis := cantorBasis[i]
		width := 1 << i

		for j := 0; j < width; j++ {
			logLUT[j+width] = logLUT[j] ^ basis
		}
	}

	for i := 0; i < order; i++ {
		logLUT[i] = expLUT[logLUT[i]]
	}

	for i := 0; i < order; i++ {
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

	for m := 0; m < bitwidth-1; m++ {
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

	for i := 0; i < modulus; i++ {
		fftSkew[i] = logLUT[fftSkew[i]]
	}

	// Precalculate FWHT(Log[i]):

	for i := 0; i < order; i++ {
		logWalsh[i] = logLUT[i]
	}
	logWalsh[0] = 0

	fwht(logWalsh, order, order)
}

func initMul16LUT() {
	mul16LUTs = &[order]mul16LUT{}

	// For each log_m multiplicand:
	for log_m := 0; log_m < order; log_m++ {
		var tmp [64]ffe
		for nibble, shift := 0, 0; nibble < 4; {
			nibble_lut := tmp[nibble*16:]

			for xnibble := 0; xnibble < 16; xnibble++ {
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
			for i := 0; i < 4; i++ {
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

func (e *errorBitfield) prepare() {
	// First mip level is for final layer of FFT: pairs of data
	for i := 0; i < kWords; i++ {
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

	for i := 0; i < kBigWords; i++ {
		w_i := uint64(0)
		bit := uint64(1)
		src := e.Words[kWordMips-1][i*64 : i*64+64]
		for _, w := range src {
			w_i |= (w | (w >> 32) | (w << 32)) & bit
			bit <<= 1
		}
		e.BigWords[0][i] = w_i

		bits := 1
		for j := 1; j < kBigMips; j++ {
			hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> bits)
			lo2hi_j := (w_i & (kHiMasks[j-1] >> bits)) << bits
			w_i = hi2lo_j | lo2hi_j
			e.BigWords[j][i] = w_i
			bits <<= 1
		}
	}

	w_i := uint64(0)
	bit := uint64(1)
	for _, w := range e.BigWords[kBigMips-1][:kBigWords] {
		w_i |= (w | (w >> 32) | (w << 32)) & bit
		bit <<= 1
	}
	e.BiggestWords[0] = w_i

	bits := uint64(1)
	for j := 1; j < kBiggestMips; j++ {
		hi2lo_j := w_i | ((w_i & kHiMasks[j-1]) >> bits)
		lo2hi_j := (w_i & (kHiMasks[j-1] >> bits)) << bits
		w_i = hi2lo_j | lo2hi_j
		e.BiggestWords[j] = w_i
		bits <<= 1
	}
}

func (e *errorBitfield) fftDIT(work [][]byte, mtrunc, m int, skewLUT []ffe, o *options) {
	// Decimation in time: Unroll 2 layers at a time
	mipLevel := bits.Len32(uint32(m)) - 1

	dist4 := m
	dist := m >> 2
	needed := e.isNeededFn(mipLevel)
	for dist != 0 {
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
		mipLevel -= 2
		needed = e.isNeededFn(mipLevel)
	}

	// If there is one layer left:
	if dist4 == 2 {
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
