// Copyright (c) 2024+ Klaus Post. See LICENSE for license

//go:build !race

package reedsolomon

const raceEnabled = false

func raceReadSlice[T any](s []T) {
}

func raceWriteSlice[T any](s []T) {
}

func raceReadSlices[T any](s [][]T, start, n int) {}

func raceWriteSlices[T any](s [][]T, start, n int) {}
