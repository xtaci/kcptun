// The MIT License (MIT)
//
// Copyright (c) 2015 xtaci
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

//go:build debug

// When the debug build tag is set, the debugLog implementation is included.
package kcp

func (kcp *KCP) debugLog(logtype KCPLogType, args ...any) {
	if kcp.logmask&logtype == 0 {
		return
	}

	var msg string
	switch logtype {
	case IKCP_LOG_OUTPUT:
		msg = "[KCP OUTPUT]"
	case IKCP_LOG_INPUT:
		msg = "[KCP INPUT]"
	case IKCP_LOG_SEND:
		msg = "[KCP SEND]"
	case IKCP_LOG_RECV:
		msg = "[KCP RECV]"
	case IKCP_LOG_OUT_ACK:
		msg = "[KCP OUTPUT ACK]"
	case IKCP_LOG_OUT_PUSH:
		msg = "[KCP OUTPUT PUSH]"
	case IKCP_LOG_OUT_WASK:
		msg = "[KCP OUTPUT WASK]"
	case IKCP_LOG_OUT_WINS:
		msg = "[KCP OUTPUT WINS]"
	case IKCP_LOG_IN_ACK:
		msg = "[KCP INPUT ACK]"
	case IKCP_LOG_IN_PUSH:
		msg = "[KCP INPUT PUSH]"
	case IKCP_LOG_IN_WASK:
		msg = "[KCP INPUT WASK]"
	case IKCP_LOG_IN_WINS:
		msg = "[KCP INPUT WINS]"
	}
	kcp.log(msg, args...)
}

// kcpTrace enables debugLog calls when the debug build tag is set.
const kcpTrace = true
