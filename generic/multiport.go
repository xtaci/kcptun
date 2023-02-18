package generic

import (
	"regexp"
	"strconv"

	"github.com/pkg/errors"
)

type MultiPort struct {
	Host    string
	MinPort uint64
	MaxPort uint64
}

// Parse mulitport listener or dialer
func ParseMultiPort(addr string) (*MultiPort, error) {
	remoteAddrMatcher := regexp.MustCompile(`(.*)\:([0-9]{1,5})-?([0-9]{1,5})?`)
	matches := remoteAddrMatcher.FindStringSubmatch(addr)

	if len(matches) >= 4 {
		var minPort, maxPort int
		minPort, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, err
		}
		maxPort = minPort

		// multiport assignment
		if matches[3] != "" {
			maxPort, err = strconv.Atoi(matches[3])
			if err != nil {
				return nil, err
			}
		}

		if (minPort > maxPort) || minPort > 65535 || maxPort > 65535 || minPort == 0 || maxPort == 0 {
			return nil, errors.Errorf("invalid port range specified: minport:%v -> maxport %v", minPort, maxPort)
		}

		mp := new(MultiPort)
		mp.Host = matches[1]
		mp.MinPort = uint64(minPort)
		mp.MaxPort = uint64(maxPort)
		return mp, nil
	}

	return nil, errors.Errorf("malformed address:%v", addr)

}
