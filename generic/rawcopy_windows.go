// +build windows

package generic

import (
	"io"
	"net"
	"syscall"
)

func rawCopy(dst io.Writer, src *net.TCPConn, ctrl *CopyControl) (written int64, err error) {
	c, err := src.SyscallConn()
	if err != nil {
		return 0, err
	}

	buf := ctrl.Buffer
	for {
		var er error
		var nr int
		rr := c.Read(func(s uintptr) bool {
			ctrl.Lock()
			defer ctrl.Unlock()
			var read uint32
			var flags uint32
			var wsabuf syscall.WSABuf
			wsabuf.Buf = &buf[0]
			wsabuf.Len = uint32(len(buf))
			er = syscall.WSARecv(syscall.Handle(s), &wsabuf, 1, &read, &flags, nil, nil)
			nr = int(read)
			return true
		})

		// read EOF
		if nr == 0 && er == nil {
			break
		}

		if nr > 0 {
			ctrl.Lock()
			nw, ew := dst.Write(buf[0:nr])
			ctrl.Unlock()
			buf = nil
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
		if rr != nil {
			if rr != io.EOF {
				err = rr
			}
			break
		}
	}

	return written, err
}
