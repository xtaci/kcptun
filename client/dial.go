package main

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	kcp "github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/tcpraw"
)

var dialCount int

func dial(config *Config, block kcp.BlockCrypt) (*kcp.UDPSession, error) {
	defer func() {
		dialCount++
	}()

	remoteAddrMatcher := regexp.MustCompile(`(.*)\:([0-9]{1,5})-?([0-9]{1,5})?`)
	matches := remoteAddrMatcher.FindStringSubmatch(config.RemoteAddr)

	var remoteAddr string
	if len(matches) == 3 { // single port
		remoteAddr = config.RemoteAddr
	} else if len(matches) == 4 { // multi port
		minPort, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, err
		}
		maxPort, err := strconv.Atoi(matches[3])
		if err != nil {
			return nil, err
		}

		if (minPort > maxPort) || minPort > 65535 || maxPort > 65535 || minPort == 0 || maxPort == 0 {
			return nil, errors.Errorf("invalid port range specified: minport:%v -> maxport %v", minPort, maxPort)
		}

		// assign remote addr
		remoteAddr = fmt.Sprintf("%v:%v", matches[1], minPort+dialCount%(maxPort-minPort+1))
	}

	if config.TCP {
		conn, err := tcpraw.Dial("tcp", remoteAddr)
		if err != nil {
			return nil, errors.Wrap(err, "tcpraw.Dial()")
		}
		return kcp.NewConn(remoteAddr, block, config.DataShard, config.ParityShard, conn)
	}
	return kcp.DialWithOptions(remoteAddr, block, config.DataShard, config.ParityShard)

}
