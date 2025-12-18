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

import "testing"

func TestParseMultiPortValid(t *testing.T) {
	tests := []struct {
		name string
		addr string
		host string
		min  uint64
		max  uint64
	}{
		{name: "SinglePort", addr: "example.com:2000", host: "example.com", min: 2000, max: 2000},
		{name: "Range", addr: "example.com:2000-2005", host: "example.com", min: 2000, max: 2005},
		{name: "IPv4Range", addr: "0.0.0.0:1-65535", host: "0.0.0.0", min: 1, max: 65535},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp, err := ParseMultiPort(tt.addr)
			if err != nil {
				t.Fatalf("ParseMultiPort(%q) unexpected error: %v", tt.addr, err)
			}

			if mp.Host != tt.host {
				t.Fatalf("expected host %q, got %q", tt.host, mp.Host)
			}

			if mp.MinPort != tt.min || mp.MaxPort != tt.max {
				t.Fatalf("expected ports [%d,%d], got [%d,%d]", tt.min, tt.max, mp.MinPort, mp.MaxPort)
			}
		})
	}
}

func TestParseMultiPortInvalid(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{name: "MissingPort", addr: "example.com"},
		{name: "ZeroPort", addr: "example.com:0"},
		{name: "PortTooLarge", addr: "example.com:70000"},
		{name: "MaxLessThanMin", addr: "example.com:3000-2000"},
		{name: "HighRange", addr: "example.com:65534-70000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseMultiPort(tt.addr); err == nil {
				t.Fatalf("ParseMultiPort(%q) expected error", tt.addr)
			}
		})
	}
}
