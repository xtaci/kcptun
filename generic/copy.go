package generic

import (
	"io"
	"sync"
)

const bufSize = 4096

type bufCache struct {
	data []byte
}

var (
	bufPool = sync.Pool{
		New: func() interface{} {
			return &bufCache{data: make([]byte, bufSize)}
		},
	}
)

// Memory optimized io.Copy function specified for this library
func Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	// If the reader has a WriteTo method, use it to do the copy.
	// Avoids an allocation and a copy.
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}
	// Similarly, if the writer has a ReadFrom method, use it to do the copy.
	if rt, ok := dst.(io.ReaderFrom); ok {
		return rt.ReadFrom(src)
	}

	// fallback to standard io.CopyBuffer
	buf := bufPool.Get().(*bufCache)
	defer bufPool.Put(buf)
	return io.CopyBuffer(dst, src, buf.data)
}
