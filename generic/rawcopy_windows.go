// +build windows

package generic

import (
	"io"
	"net"
)

func rawCopy(dst io.Writer, src *net.TCPConn, ctrl *CopyControl) (written int64, err error) {
	// fallback to standard io.CopyBuffer
	buf := make([]byte, bufSize)
	return io.CopyBuffer(dst, src, buf)
}
