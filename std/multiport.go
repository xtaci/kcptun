// The MIT License (MIT)
//
// Copyright (c) 2016 xtaci
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
	"net"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type MultiPort struct {
	Host    string
	MinPort uint64
	MaxPort uint64
}

// ParseMultiPort parses a multiport listener or dialer address.
func ParseMultiPort(addr string) (*MultiPort, error) {
	host, portRange, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, errors.Wrapf(err, "malformed address %q", addr)
	}

	minText, maxText, hasRange := strings.Cut(portRange, "-")
	if !hasRange {
		maxText = minText
	} else if strings.Contains(maxText, "-") {
		return nil, errors.Errorf("invalid port range specified: %q", portRange)
	}

	parsePort := func(text string) (uint64, error) {
		if text == "" {
			return 0, errors.Errorf("empty port in address %q", addr)
		}
		for i := 0; i < len(text); i++ {
			if text[i] < '0' || text[i] > '9' {
				return 0, errors.Errorf("invalid port %q in address %q", text, addr)
			}
		}

		port, err := strconv.ParseUint(text, 10, 16)
		if err != nil || port == 0 {
			return 0, errors.Errorf("invalid port %q in address %q", text, addr)
		}
		return port, nil
	}

	minPort, err := parsePort(minText)
	if err != nil {
		return nil, err
	}
	maxPort, err := parsePort(maxText)
	if err != nil {
		return nil, err
	}
	if minPort > maxPort {
		return nil, errors.Errorf("invalid port range specified: minport:%v -> maxport %v", minPort, maxPort)
	}

	return &MultiPort{
		Host:    host,
		MinPort: minPort,
		MaxPort: maxPort,
	}, nil
}
