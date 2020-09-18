// Copyright (c) 2019. Temple3x (temple3x@gmail.com)
//
// Use of this source code is governed by the MIT License
// that can be found in the LICENSE file.
//
// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !amd64

package xorsimd

import (
	"runtime"
	"unsafe"
)

const wordSize = int(unsafe.Sizeof(uintptr(0)))
const supportsUnaligned = runtime.GOARCH == "386" || runtime.GOARCH == "ppc64" || runtime.GOARCH == "ppc64le" || runtime.GOARCH == "s390x"

func encode(dst []byte, src [][]byte) {
	if supportsUnaligned {
		fastEncode(dst, src, len(dst))
	} else {
		// TODO(hanwen): if (dst, a, b) have common alignment
		// we could still try fastEncode. It is not clear
		// how often this happens, and it's only worth it if
		// the block encryption itself is hardware
		// accelerated.
		safeEncode(dst, src, len(dst))
	}

}

// fastEncode xor in bulk. It only works on architectures that
// support unaligned read/writes.
func fastEncode(dst []byte, src [][]byte, n int) {
	w := n / wordSize
	if w > 0 {
		wordBytes := w * wordSize

		wordAlignSrc := make([][]byte, len(src))
		for i := range src {
			wordAlignSrc[i] = src[i][:wordBytes]
		}
		fastEnc(dst[:wordBytes], wordAlignSrc)
	}

	for i := n - n%wordSize; i < n; i++ {
		s := src[0][i]
		for j := 1; j < len(src); j++ {
			s ^= src[j][i]
		}
		dst[i] = s
	}
}

func fastEnc(dst []byte, src [][]byte) {
	dw := *(*[]uintptr)(unsafe.Pointer(&dst))
	sw := make([][]uintptr, len(src))
	for i := range src {
		sw[i] = *(*[]uintptr)(unsafe.Pointer(&src[i]))
	}

	n := len(dst) / wordSize
	for i := 0; i < n; i++ {
		s := sw[0][i]
		for j := 1; j < len(sw); j++ {
			s ^= sw[j][i]
		}
		dw[i] = s
	}
}

func safeEncode(dst []byte, src [][]byte, n int) {
	for i := 0; i < n; i++ {
		s := src[0][i]
		for j := 1; j < len(src); j++ {
			s ^= src[j][i]
		}
		dst[i] = s
	}
}

// Bytes8 XORs of word 8 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 8,
// if not, Bytes8 will panic.
func Bytes8(dst, a, b []byte) {

	bytesWords(dst[:8], a[:8], b[:8])
}

// Bytes16 XORs of packed doubleword 16 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 16,
// if not, Bytes16 will panic.
func Bytes16(dst, a, b []byte) {

	bytesWords(dst[:16], a[:16], b[:16])
}

// bytesWords XORs multiples of 4 or 8 bytes (depending on architecture.)
// The slice arguments a and b are assumed to be of equal length.
func bytesWords(dst, a, b []byte) {
	if supportsUnaligned {
		dw := *(*[]uintptr)(unsafe.Pointer(&dst))
		aw := *(*[]uintptr)(unsafe.Pointer(&a))
		bw := *(*[]uintptr)(unsafe.Pointer(&b))
		n := len(b) / wordSize
		for i := 0; i < n; i++ {
			dw[i] = aw[i] ^ bw[i]
		}
	} else {
		n := len(b)
		for i := 0; i < n; i++ {
			dst[i] = a[i] ^ b[i]
		}
	}
}

// Bytes8Align XORs of 8 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 8,
// if not, Bytes8 will panic.
//
// All the byte slices must be aligned to wordsize.
func Bytes8Align(dst, a, b []byte) {

	bytesWordsAlign(dst[:8], a[:8], b[:8])
}

// Bytes16Align XORs of packed 16 Bytes.
// The slice arguments a, b, dst's lengths are assumed to be at least 16,
// if not, Bytes16 will panic.
//
// All the byte slices must be aligned to wordsize.
func Bytes16Align(dst, a, b []byte) {

	bytesWordsAlign(dst[:16], a[:16], b[:16])
}

// bytesWordsAlign XORs multiples of 4 or 8 bytes (depending on architecture.)
// The slice arguments a and b are assumed to be of equal length.
//
// All the byte slices must be aligned to wordsize.
func bytesWordsAlign(dst, a, b []byte) {
	dw := *(*[]uintptr)(unsafe.Pointer(&dst))
	aw := *(*[]uintptr)(unsafe.Pointer(&a))
	bw := *(*[]uintptr)(unsafe.Pointer(&b))
	n := len(b) / wordSize
	for i := 0; i < n; i++ {
		dw[i] = aw[i] ^ bw[i]
	}
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

	n := len(a)
	bytesN(dst[:n], a[:n], b[:n], n)
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

	n := len(b)
	bytesN(dst[:n], a[:n], b[:n], n)
}

func bytesN(dst, a, b []byte, n int) {

	switch {
	case supportsUnaligned:
		w := n / wordSize
		if w > 0 {
			dw := *(*[]uintptr)(unsafe.Pointer(&dst))
			aw := *(*[]uintptr)(unsafe.Pointer(&a))
			bw := *(*[]uintptr)(unsafe.Pointer(&b))
			for i := 0; i < w; i++ {
				dw[i] = aw[i] ^ bw[i]
			}
		}

		for i := (n - n%wordSize); i < n; i++ {
			dst[i] = a[i] ^ b[i]
		}
	default:
		for i := 0; i < n; i++ {
			dst[i] = a[i] ^ b[i]
		}
	}
}
