// Copyright (c) 2019. Temple3x (temple3x@gmail.com)
//
// Use of this source code is governed by the MIT License
// that can be found in the LICENSE file.

package xorsimd

import "github.com/templexxx/cpu"

// EnableAVX512 may slow down CPU Clock (maybe not).
// TODO need more research:
// https://lemire.me/blog/2018/04/19/by-how-much-does-avx-512-slow-down-your-cpu-a-first-experiment/
var EnableAVX512 = true

// cpuFeature indicates which instruction set will be used.
var cpuFeature = getCPUFeature()

const (
	avx512 = iota
	avx2
	sse2
	generic
)

// TODO: Add ARM feature...
func getCPUFeature() int {
	if hasAVX512() && EnableAVX512 {
		return avx512
	} else if cpu.X86.HasAVX2 {
		return avx2
	} else {
		return sse2 // amd64 must has sse2
	}
}

func hasAVX512() (ok bool) {

	return cpu.X86.HasAVX512VL &&
		cpu.X86.HasAVX512BW &&
		cpu.X86.HasAVX512F &&
		cpu.X86.HasAVX512DQ
}

// Encode encodes elements from source slice into a
// destination slice. The source and destination may overlap.
// Encode returns the number of bytes encoded, which will be the minimum of
// len(src[i]) and len(dst).
func Encode(dst []byte, src [][]byte) (n int) {
	n = checkLen(dst, src)
	if n == 0 {
		return
	}

	dst = dst[:n]
	for i := range src {
		src[i] = src[i][:n]
	}

	if len(src) == 1 {
		copy(dst, src[0])
		return
	}

	encode(dst, src)
	return
}

func checkLen(dst []byte, src [][]byte) int {
	n := len(dst)
	for i := range src {
		if len(src[i]) < n {
			n = len(src[i])
		}
	}

	if n <= 0 {
		return 0
	}
	return n
}

// Bytes XORs the bytes in a and b into a
// destination slice. The source and destination may overlap.
//
// Bytes returns the number of bytes encoded, which will be the minimum of
// len(dst), len(a), len(b).
func Bytes(dst, a, b []byte) int {
	return Encode(dst, [][]byte{a, b})
}
