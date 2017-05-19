// +build !android

package main

type Callback func(int, int)

func SetNetCallback(callback Callback) {
}
