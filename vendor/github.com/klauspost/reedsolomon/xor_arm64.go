//go:build !noasm && !appengine && !gccgo

package reedsolomon

//go:noescape
func xorSliceNEON(in, out []byte)

// simple slice xor
func sliceXor(in, out []byte, o *options) {
	done := (len(in) >> 5) << 5
	if raceEnabled {
		raceWriteSlice(out[:done])
		raceReadSlice(in[:done])
	}
	xorSliceNEON(in, out)

	remain := len(in) - done
	if remain > 0 {
		for i := done; i < len(in); i++ {
			out[i] ^= in[i]
		}
	}
}
