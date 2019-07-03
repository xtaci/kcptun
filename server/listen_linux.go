// +build linux

package main

import (
	"github.com/pkg/errors"
	kcp "github.com/xtaci/kcp-go"
	"github.com/xtaci/tcpraw"
)

func listen(config *Config, block kcp.BlockCrypt) (*kcp.Listener, error) {
	if config.TCP {
		conn, err := tcpraw.Listen("tcp", config.Listen)
		if err != nil {
			return nil, errors.Wrap(err, "tcpraw.Listen()")
		}
		return kcp.ServeConn(block, config.DataShard, config.ParityShard, conn)
	}
	return kcp.ListenWithOptions(config.Listen, block, config.DataShard, config.ParityShard)
}
