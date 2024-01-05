//go:build noasm || gccgo || appengine || (!amd64 && !arm64)

package reedsolomon

func sliceXor(in, out []byte, o *options) {
	sliceXorGo(in, out, o)
}
