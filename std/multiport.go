// The MIT License (MIT)
//
// # Copyright (c) 2016 xtaci
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package std

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
