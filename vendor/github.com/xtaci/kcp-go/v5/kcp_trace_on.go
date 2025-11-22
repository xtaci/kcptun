//go:build debug

// only build tag debug is set, then debugLog will be enabled in compile time
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
