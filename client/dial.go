// +build !linux

package main

import (
	kcp "github.com/xtaci/kcp-go"
)

func dial(config *Config, block kcp.BlockCrypt) (*kcp.UDPSession, error) {
	return kcp.DialWithOptions(config.RemoteAddr, block, config.DataShard, config.ParityShard)
}
