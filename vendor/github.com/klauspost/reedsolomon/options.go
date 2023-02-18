package reedsolomon

import (
	"runtime"

	"github.com/klauspost/cpuid/v2"
)

// Option allows to override processing parameters.
type Option func(*options)

type options struct {
	maxGoroutines int
	minSplitSize  int
	shardSize     int
	perRound      int

	useAVX512, useAVX2, useSSSE3, useSSE2 bool
	usePAR1Matrix                         bool
	useCauchy                             bool
	fastOneParity                         bool
	inversionCache                        bool
	customMatrix                          [][]byte

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
	useSSSE3:  cpuid.CPU.Supports(cpuid.SSSE3),
	useSSE2:   cpuid.CPU.Supports(cpuid.SSE2),
	useAVX2:   cpuid.CPU.Supports(cpuid.AVX2),
	useAVX512: cpuid.CPU.Supports(cpuid.AVX512F, cpuid.AVX512BW),
}

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
// Enabled by default.
func WithInversionCache(enabled bool) Option {
	return func(o *options) {
		o.inversionCache = enabled
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
// If not set, AVX2 will be turned on or off automatically based on CPU ID information.
func WithAVX2(enabled bool) Option {
	return func(o *options) {
		o.useAVX2 = enabled
	}
}

// WithSSE2 allows to enable/disable SSE2 instructions.
// If not set, SSE2 will be turned on or off automatically based on CPU ID information.
func WithSSE2(enabled bool) Option {
	return func(o *options) {
		o.useSSE2 = enabled
	}
}

// WithAVX512 allows to enable/disable AVX512 instructions.
// If not set, AVX512 will be turned on or off automatically based on CPU ID information.
func WithAVX512(enabled bool) Option {
	return func(o *options) {
		o.useAVX512 = enabled
	}
}

// WithPAR1Matrix causes the encoder to build the matrix how PARv1
// does. Note that the method they use is buggy, and may lead to cases
// where recovery is impossible, even if there are enough parity
// shards.
func WithPAR1Matrix() Option {
	return func(o *options) {
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
		o.useCauchy = true
		o.usePAR1Matrix = false
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
