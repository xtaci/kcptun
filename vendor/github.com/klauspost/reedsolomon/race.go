// Copyright (c) 2024+ Klaus Post. See LICENSE for license

//go:build race

package reedsolomon

import (
	"runtime"
	"unsafe"
)

const raceEnabled = true

func raceReadSlice[T any](s []T) {
	if len(s) == 0 {
		return
	}
	runtime.RaceReadRange(unsafe.Pointer(&s[0]), len(s)*int(unsafe.Sizeof(s[0])))
}

func raceWriteSlice[T any](s []T) {
	if len(s) == 0 {
		return
	}
	runtime.RaceWriteRange(unsafe.Pointer(&s[0]), len(s)*int(unsafe.Sizeof(s[0])))
}

func raceReadSlices[T any](s [][]T, start, n int) {
	if len(s) == 0 {
		return
	}
	runtime.RaceReadRange(unsafe.Pointer(&s[0]), len(s)*int(unsafe.Sizeof(s[0])))
	for _, v := range s {
		if len(v) == 0 {
			continue
		}
		n := n
		if n < 0 {
			n = len(v) - start
		}
		runtime.RaceReadRange(unsafe.Pointer(&v[start]), n*int(unsafe.Sizeof(v[0])))
	}
}

func raceWriteSlices[T any](s [][]T, start, n int) {
	if len(s) == 0 {
		return
	}
	runtime.RaceReadRange(unsafe.Pointer(&s[0]), len(s)*int(unsafe.Sizeof(s[0])))

	for _, v := range s {
		if len(v) == 0 {
			continue
		}
		n := n
		if n < 0 {
			n = len(v) - start
		}
		runtime.RaceWriteRange(unsafe.Pointer(&v[start]), n*int(unsafe.Sizeof(v[0])))
	}
}
