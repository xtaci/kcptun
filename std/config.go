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
	"encoding/json"
	"os"
)

// BaseConfig contains shared configuration fields between client and server.
// Embedding this struct reduces code duplication and ensures consistency.
type BaseConfig struct {
	Key          string `json:"key"`
	Crypt        string `json:"crypt"`
	Mode         string `json:"mode"`
	MTU          int    `json:"mtu"`
	RateLimit    int    `json:"ratelimit"`
	SndWnd       int    `json:"sndwnd"`
	RcvWnd       int    `json:"rcvwnd"`
	DataShard    int    `json:"datashard"`
	ParityShard  int    `json:"parityshard"`
	DSCP         int    `json:"dscp"`
	NoComp       bool   `json:"nocomp"`
	AckNodelay   bool   `json:"acknodelay"`
	NoDelay      int    `json:"nodelay"`
	Interval     int    `json:"interval"`
	Resend       int    `json:"resend"`
	NoCongestion int    `json:"nc"`
	SockBuf      int    `json:"sockbuf"`
	SmuxVer      int    `json:"smuxver"`
	SmuxBuf      int    `json:"smuxbuf"`
	FrameSize    int    `json:"framesize"`
	StreamBuf    int    `json:"streambuf"`
	KeepAlive    int    `json:"keepalive"`
	Log          string `json:"log"`
	SnmpLog      string `json:"snmplog"`
	SnmpPeriod   int    `json:"snmpperiod"`
	Quiet        bool   `json:"quiet"`
	TCP          bool   `json:"tcp"`
	Pprof        bool   `json:"pprof"`
	QPP          bool   `json:"qpp"`
	QPPCount     int    `json:"qpp-count"`
	CloseWait    int    `json:"closewait"`
}

// ModeParams contains the KCP parameters for different transmission modes.
type ModeParams struct {
	NoDelay      int
	Interval     int
	Resend       int
	NoCongestion int
}

// PredefinedModes maps mode names to their KCP parameters.
// Using a map simplifies mode selection and makes adding new modes easier.
var PredefinedModes = map[string]ModeParams{
	"normal": {0, 40, 2, 1},
	"fast":   {0, 30, 2, 1},
	"fast2":  {1, 20, 2, 1},
	"fast3":  {1, 10, 2, 1},
}

// ApplyMode sets the KCP parameters based on the mode name.
// Returns true if the mode was found and applied.
func (c *BaseConfig) ApplyMode() bool {
	if params, ok := PredefinedModes[c.Mode]; ok {
		c.NoDelay = params.NoDelay
		c.Interval = params.Interval
		c.Resend = params.Resend
		c.NoCongestion = params.NoCongestion
		return true
	}
	return false
}

// ParseJSONConfig loads configuration from a JSON file.
func ParseJSONConfig(config interface{}, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(config)
}
