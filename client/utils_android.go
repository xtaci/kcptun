// +build android

package main

import (
	"net"
)

func SetNetCallback(callback Callback) {
	net.Callback = callback
}
