// +build !android

package main

import "github.com/xtaci/kcp-go"

func DialKCP(raddr string, block kcp.BlockCrypt, dataShards, parityShards int) (*kcp.UDPSession, error) {
    return kcp.DialWithOptions(raddr, block, dataShards, parityShards)
}
