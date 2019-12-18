// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package generic

import (
	"io"
	"net"

	"syscall"
)

// rawCopy can fan in N src into 1 dst with only 1 shared buffer
func rawCopy(dst io.Writer, src *net.TCPConn, ctrl *CopyControl) (written int64, err error) {
	c, err := src.SyscallConn()
	if err != nil {
		return 0, err
	}

	buf := ctrl.Buffer
	var locked bool
	for {
		var er error
		var nr int
		rr := c.Read(func(s uintptr) bool {
			// if the 'src' readable, acquire the shared lock first
			// to make sure no other writers to 'dst' are blocked on dst.Write.
			// With such design, we only need 1 buffer for a specific 'dst',
			// especially when 'dst' is a multiplexed connection.
			ctrl.Lock()
			locked = true
			nr, er = syscall.Read(int(s), buf)
			if er == syscall.EAGAIN {
				ctrl.Unlock()
				locked = false
				return false
			}
			// keep the lock on the shared buffer
			// for the following dst.Write
			return true
		})

		// read EOF
		if nr == 0 && er == nil {
			break
		}

		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			ctrl.Unlock()
			locked = false

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

	if locked {
		ctrl.Unlock()
	}

	return written, err
}
