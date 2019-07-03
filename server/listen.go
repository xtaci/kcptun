// +build !linux

package main

import kcp "github.com/xtaci/kcp-go"

func listen(config *Config, block kcp.BlockCrypt) (*kcp.Listener, error) {
	return kcp.ListenWithOptions(config.Listen, block, config.DataShard, config.ParityShard)
}
