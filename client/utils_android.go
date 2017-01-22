// +build android

package main

import (
	"net"
)

func SetNetCallback(callback net.DialCallback) {
	net.Callback = callback
}
