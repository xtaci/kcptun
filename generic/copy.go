package generic

import (
	"io"
	"net"
	"sync"
)

const bufSize = 4096

type CopyControl struct {
	Buffer []byte // shared buffer for copying controlled by mutex
	sync.Mutex
}

// Memory optimized io.Copy function specified for this library
func Copy(dst io.Writer, src io.Reader, ctrl *CopyControl) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}

	// if src is net.TCPConn, and dst is a multiplexed connection
	// reading can be controlled by writable events of smux
	// and make the reading serialized
	if tcpconn, ok := src.(*net.TCPConn); ok {
		if ctrl != nil {
			return rawCopy(dst, tcpconn, ctrl)
		}
	}

	// fallback to standard io.CopyBuffer
	buf := make([]byte, bufSize)
	return io.CopyBuffer(dst, src, buf)
}
