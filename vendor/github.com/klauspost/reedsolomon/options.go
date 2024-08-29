package reedsolomon

import (
	"runtime"
	"strings"

	"github.com/klauspost/cpuid/v2"
)

// Option allows to override processing parameters.
type Option func(*options)

type options struct {
	maxGoroutines int
	minSplitSize  int
	shardSize     int
	perRound      int

	useAvxGNFI,
	useAvx512GFNI,
	useAVX512,
	useAVX2,
	useSSSE3,
	useSSE2,
	useNEON,
	useSVE bool

	useJerasureMatrix    bool
	usePAR1Matrix        bool
	useCauchy            bool
	fastOneParity        bool
	inversionCache       bool
	forcedInversionCache bool
	customMatrix         [][]byte
	withLeopard          leopardMode

	// stream options
	concReads  bool
	concWrites bool
	streamBS   int
}

var defaultOptions = options{
	maxGoroutines:  384,
	minSplitSize:   -1,
	fastOneParity:  false,
	inversionCache: true,

	// Detect CPU capabilities.
	useSSSE3:      cpuid.CPU.Supports(cpuid.SSSE3),
	useSSE2:       cpuid.CPU.Supports(cpuid.SSE2),
	useAVX2:       cpuid.CPU.Supports(cpuid.AVX2),
	useAVX512:     cpuid.CPU.Supports(cpuid.AVX512F, cpuid.AVX512BW, cpuid.AVX512VL),
	useAvx512GFNI: cpuid.CPU.Supports(cpuid.AVX512F, cpuid.GFNI, cpuid.AVX512DQ),
	useAvxGNFI:    cpuid.CPU.Supports(cpuid.AVX, cpuid.GFNI),
	useNEON:       cpuid.CPU.Supports(cpuid.ASIMD),
	useSVE:        cpuid.CPU.Supports(cpuid.SVE),
}

// leopardMode controls the use of leopard GF in encoding and decoding.
type leopardMode int

const (
	// leopardAsNeeded only switches to leopard 16-bit when there are more than
	// 256 shards.
	leopardAsNeeded leopardMode = iota
	// leopardGF16 uses leopard in 16-bit mode for all shard counts.
	leopardGF16
	// leopardAlways uses 8-bit leopard for shards less than or equal to 256,
	// 16-bit leopard otherwise.
	leopardAlways
)

func init() {
	if runtime.GOMAXPROCS(0) <= 1 {
		defaultOptions.maxGoroutines = 1
	}
}

// WithMaxGoroutines is the maximum number of goroutines number for encoding & decoding.
// Jobs will be split into this many parts, unless each goroutine would have to process
// less than minSplitSize bytes (set with WithMinSplitSize).
// For the best speed, keep this well above the GOMAXPROCS number for more fine grained
// scheduling.
// If n <= 0, it is ignored.
func WithMaxGoroutines(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.maxGoroutines = n
		}
	}
}

// WithAutoGoroutines will adjust the number of goroutines for optimal speed with a
// specific shard size.
// Send in the shard size you expect to send. Other shard sizes will work, but may not
// run at the optimal speed.
// Overwrites WithMaxGoroutines.
// If shardSize <= 0, it is ignored.
func WithAutoGoroutines(shardSize int) Option {
	return func(o *options) {
		o.shardSize = shardSize
	}
}

// WithMinSplitSize is the minimum encoding size in bytes per goroutine.
// By default this parameter is determined by CPU cache characteristics.
// See WithMaxGoroutines on how jobs are split.
// If n <= 0, it is ignored.
func WithMinSplitSize(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.minSplitSize = n
		}
	}
}

// WithConcurrentStreams will enable concurrent reads and writes on the streams.
// Default: Disabled, meaning only one stream will be read/written at the time.
// Ignored if not used on a stream input.
func WithConcurrentStreams(enabled bool) Option {
	return func(o *options) {
		o.concReads, o.concWrites = enabled, enabled
	}
}

// WithConcurrentStreamReads will enable concurrent reads from the input streams.
// Default: Disabled, meaning only one stream will be read at the time.
// Ignored if not used on a stream input.
func WithConcurrentStreamReads(enabled bool) Option {
	return func(o *options) {
		o.concReads = enabled
	}
}

// WithConcurrentStreamWrites will enable concurrent writes to the the output streams.
// Default: Disabled, meaning only one stream will be written at the time.
// Ignored if not used on a stream input.
func WithConcurrentStreamWrites(enabled bool) Option {
	return func(o *options) {
		o.concWrites = enabled
	}
}

// WithInversionCache allows to control the inversion cache.
// This will cache reconstruction matrices so they can be reused.
// Enabled by default, or <= 64 shards for Leopard encoding.
func WithInversionCache(enabled bool) Option {
	return func(o *options) {
		o.inversionCache = enabled
		o.forcedInversionCache = true
	}
}

// WithStreamBlockSize allows to set a custom block size per round of reads/writes.
// If not set, any shard size set with WithAutoGoroutines will be used.
// If WithAutoGoroutines is also unset, 4MB will be used.
// Ignored if not used on stream.
func WithStreamBlockSize(n int) Option {
	return func(o *options) {
		o.streamBS = n
	}
}

// WithSSSE3 allows to enable/disable SSSE3 instructions.
// If not set, SSSE3 will be turned on or off automatically based on CPU ID information.
func WithSSSE3(enabled bool) Option {
	return func(o *options) {
		o.useSSSE3 = enabled
	}
}

// WithAVX2 allows to enable/disable AVX2 instructions.
// If not set, AVX will be turned on or off automatically based on CPU ID information.
// This will also disable AVX GFNI instructions.
func WithAVX2(enabled bool) Option {
	return func(o *options) {
		o.useAVX2 = enabled
		if o.useAvxGNFI {
			o.useAvxGNFI = enabled
		}
	}
}

// WithSSE2 allows to enable/disable SSE2 instructions.
// If not set, SSE2 will be turned on or off automatically based on CPU ID information.
func WithSSE2(enabled bool) Option {
	return func(o *options) {
		o.useSSE2 = enabled
	}
}

// WithAVX512 allows to enable/disable AVX512 (and GFNI) instructions.
func WithAVX512(enabled bool) Option {
	return func(o *options) {
		o.useAVX512 = enabled
		o.useAvx512GFNI = enabled
	}
}

// WithGFNI allows to enable/disable AVX512+GFNI instructions.
// If not set, GFNI will be turned on or off automatically based on CPU ID information.
func WithGFNI(enabled bool) Option {
	return func(o *options) {
		o.useAvx512GFNI = enabled
	}
}

// WithAVXGFNI allows to enable/disable GFNI with AVX instructions.
// If not set, GFNI will be turned on or off automatically based on CPU ID information.
func WithAVXGFNI(enabled bool) Option {
	return func(o *options) {
		o.useAvxGNFI = enabled
	}
}

// WithJerasureMatrix causes the encoder to build the Reed-Solomon-Vandermonde
// matrix in the same way as done by the Jerasure library.
// The first row and column of the coding matrix only contains 1's in this method
// so the first parity chunk is always equal to XOR of all data chunks.
func WithJerasureMatrix() Option {
	return func(o *options) {
		o.useJerasureMatrix = true
		o.usePAR1Matrix = false
		o.useCauchy = false
	}
}

// WithPAR1Matrix causes the encoder to build the matrix how PARv1
// does. Note that the method they use is buggy, and may lead to cases
// where recovery is impossible, even if there are enough parity
// shards.
func WithPAR1Matrix() Option {
	return func(o *options) {
		o.useJerasureMatrix = false
		o.usePAR1Matrix = true
		o.useCauchy = false
	}
}

// WithCauchyMatrix will make the encoder build a Cauchy style matrix.
// The output of this is not compatible with the standard output.
// A Cauchy matrix is faster to generate. This does not affect data throughput,
// but will result in slightly faster start-up time.
func WithCauchyMatrix() Option {
	return func(o *options) {
		o.useJerasureMatrix = false
		o.usePAR1Matrix = false
		o.useCauchy = true
	}
}

// WithFastOneParityMatrix will switch the matrix to a simple xor
// if there is only one parity shard.
// The PAR1 matrix already has this property so it has little effect there.
func WithFastOneParityMatrix() Option {
	return func(o *options) {
		o.fastOneParity = true
	}
}

// WithCustomMatrix causes the encoder to use the manually specified matrix.
// customMatrix represents only the parity chunks.
// customMatrix must have at least ParityShards rows and DataShards columns.
// It can be used for interoperability with libraries which generate
// the matrix differently or to implement more complex coding schemes like LRC
// (locally reconstructible codes).
func WithCustomMatrix(customMatrix [][]byte) Option {
	return func(o *options) {
		o.customMatrix = customMatrix
	}
}

// WithLeopardGF16 will always use leopard GF16 for encoding,
// even when there is less than 256 shards.
// This will likely improve reconstruction time for some setups.
// This is not compatible with Leopard output for <= 256 shards.
// Note that Leopard places certain restrictions on use see other documentation.
func WithLeopardGF16(enabled bool) Option {
	return func(o *options) {
		if enabled {
			o.withLeopard = leopardGF16
		} else {
			o.withLeopard = leopardAsNeeded
		}
	}
}

// WithLeopardGF will use leopard GF for encoding, even when there are fewer than
// 256 shards.
// This will likely improve reconstruction time for some setups.
// Note that Leopard places certain restrictions on use see other documentation.
func WithLeopardGF(enabled bool) Option {
	return func(o *options) {
		if enabled {
			o.withLeopard = leopardAlways
		} else {
			o.withLeopard = leopardAsNeeded
		}
	}
}

func (o *options) cpuOptions() string {
	var res []string
	if o.useSSE2 {
		res = append(res, "SSE2")
	}
	if o.useAVX2 {
		res = append(res, "AVX2")
	}
	if o.useSSSE3 {
		res = append(res, "SSSE3")
	}
	if o.useAVX512 {
		res = append(res, "AVX512")
	}
	if o.useAvx512GFNI {
		res = append(res, "AVX512+GFNI")
	}
	if o.useAvxGNFI {
		res = append(res, "AVX+GFNI")
	}
	if o.useSVE {
		res = append(res, "ARM+SVE")
	} else if o.useNEON {
		res = append(res, "ARM+NEON")
	}
	if len(res) == 0 {
		return "pure Go"
	}
	return strings.Join(res, ",")
}
