// Copyright (c) 2019. Temple3x (temple3x@gmail.com)
//
// Use of this source code is governed by the MIT License
// that can be found in the LICENSE file.

package xorsimd

func encode(dst []byte, src [][]byte) {

	switch cpuFeature {
	case avx512:
		encodeAVX512(dst, src)
	case avx2:
		encodeAVX2(dst, src)
	default:
		encodeSSE2(dst, src)
	}
	return
}

// Bytes8 XORs of 8 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 8,
// if not, Bytes8 will panic.
func Bytes8(dst, a, b []byte) {

	bytes8(&dst[0], &a[0], &b[0])
}

// Bytes16 XORs of packed 16 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 16,
// if not, Bytes16 will panic.
func Bytes16(dst, a, b []byte) {

	bytes16(&dst[0], &a[0], &b[0])
}

// Bytes8Align XORs of 8 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 8,
// if not, Bytes8 will panic.
func Bytes8Align(dst, a, b []byte) {

	bytes8(&dst[0], &a[0], &b[0])
}

// Bytes16Align XORs of packed 16 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 16,
// if not, Bytes16 will panic.
func Bytes16Align(dst, a, b []byte) {

	bytes16(&dst[0], &a[0], &b[0])
}

// BytesA XORs the len(a) bytes in a and b into a
// destination slice.
// The destination should have enough space.
//
// It's used for encoding small bytes slices (< dozens bytes),
// and the slices may not be aligned to 8 bytes or 16 bytes.
// If the length is big, it's better to use 'func Bytes(dst, a, b []byte)' instead
// for gain better performance.
func BytesA(dst, a, b []byte) {

	bytesN(&dst[0], &a[0], &b[0], len(a))
}

// BytesB XORs the len(b) bytes in a and b into a
// destination slice.
// The destination should have enough space.
//
// It's used for encoding small bytes slices (< dozens bytes),
// and the slices may not be aligned to 8 bytes or 16 bytes.
// If the length is big, it's better to use 'func Bytes(dst, a, b []byte)' instead
// for gain better performance.
func BytesB(dst, a, b []byte) {

	bytesN(&dst[0], &a[0], &b[0], len(b))
}

//go:noescape
func encodeAVX512(dst []byte, src [][]byte)

//go:noescape
func encodeAVX2(dst []byte, src [][]byte)

//go:noescape
func encodeSSE2(dst []byte, src [][]byte)

//go:noescape
func bytesN(dst, a, b *byte, n int)

//go:noescape
func bytes8(dst, a, b *byte)

//go:noescape
func bytes16(dst, a, b *byte)
