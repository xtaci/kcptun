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
	"time"

	"github.com/xtaci/smux"
)

// BuildSmuxConfig constructs a smux.Config from CLI parameters and verifies the
// result. Callers can log or wrap the returned error for better diagnostics.
func BuildSmuxConfig(version, maxReceiveBuffer, maxStreamBuffer, maxFrameSize, keepAliveSeconds int) (*smux.Config, error) {
	cfg := smux.DefaultConfig()
	cfg.Version = version
	cfg.MaxReceiveBuffer = maxReceiveBuffer
	cfg.MaxStreamBuffer = maxStreamBuffer
	cfg.MaxFrameSize = maxFrameSize
	cfg.KeepAliveInterval = time.Duration(keepAliveSeconds) * time.Second

	return cfg, smux.VerifyConfig(cfg)
}
