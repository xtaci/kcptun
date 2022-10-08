package main

import (
	"fmt"

	"github.com/pkg/errors"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/kcptun/generic"
	"github.com/xtaci/tcpraw"
)

var dialCount uint64

func dial(config *Config, block kcp.BlockCrypt) (*kcp.UDPSession, error) {
	defer func() {
		dialCount++
	}()

	mp, err := generic.ParseMultiPort(config.RemoteAddr)
	if err != nil {
		return nil, err
	}

	remoteAddr := fmt.Sprintf("%v:%v", mp.Host, uint64(mp.MinPort)+dialCount%uint64(mp.MaxPort-mp.MinPort+1))

	if config.TCP {
		conn, err := tcpraw.Dial("tcp", remoteAddr)
		if err != nil {
			return nil, errors.Wrap(err, "tcpraw.Dial()")
		}
		return kcp.NewConn(remoteAddr, block, config.DataShard, config.ParityShard, conn)
	}
	return kcp.DialWithOptions(remoteAddr, block, config.DataShard, config.ParityShard)

}
