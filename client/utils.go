// +build !android

package main

type Callback func(int)

func SetNetCallback(callback Callback) {
}
