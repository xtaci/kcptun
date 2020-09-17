package main

import (
	"github.com/pkg/errors"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/tcpraw"
	"net"
)

func dial(config *Config, block kcp.BlockCrypt) (*kcp.UDPSession, error) {
	if config.TCP {
		conn, err := tcpraw.Dial("tcp", config.RemoteAddr)
		if err != nil {
			return nil, errors.Wrap(err, "tcpraw.Dial()")
		}
		return kcp.NewConn(config.RemoteAddr, block, config.DataShard, config.ParityShard, conn)
	}
	if config.BindUDP != "" {
		//log.Println("func dial() config.BindUDP = ", config.BindUDP)
		return dialAndBindUdp(config, block)
	}
	return kcp.DialWithOptions(config.RemoteAddr, block, config.DataShard, config.ParityShard)
}

func dialAndBindUdp(config *Config, block kcp.BlockCrypt) (*kcp.UDPSession, error) {
	udpaddr, err := net.ResolveUDPAddr("udp", config.BindUDP)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	network := "udp4"
	if udpaddr.IP.To4() == nil {
		network = "udp"
	}
	conn, err := net.ListenUDP(network, udpaddr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return kcp.NewConn(config.RemoteAddr, block, config.DataShard, config.ParityShard, conn)
}
