package memcall

import (
	"reflect"
	"runtime"
	"unsafe"
)

// MemoryProtectionFlag specifies some particular memory protection flag.
type MemoryProtectionFlag struct {
	// NOACCESS  := 1 (0001)
	// READ      := 2 (0010)
	// WRITE     := 4 (0100) // unused
	// READWRITE := 6 (0110)

	flag byte
}

// NoAccess specifies that the memory should be marked unreadable and immutable.
func NoAccess() MemoryProtectionFlag {
	return MemoryProtectionFlag{1}
}

// ReadOnly specifies that the memory should be marked read-only (immutable).
func ReadOnly() MemoryProtectionFlag {
	return MemoryProtectionFlag{2}
}

// ReadWrite specifies that the memory should be made readable and writable.
func ReadWrite() MemoryProtectionFlag {
	return MemoryProtectionFlag{6}
}

// ErrInvalidFlag indicates that a given memory protection flag is undefined.
const ErrInvalidFlag = "<memcall> memory protection flag is undefined"

// Wipes a given byte slice.
func wipe(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
	runtime.KeepAlive(buf)
}

// Placeholder variable for when we need a valid pointer to zero bytes.
var _zero uintptr

// Auxiliary functions.
func _getStartPtr(b []byte) unsafe.Pointer {
	if len(b) > 0 {
		return unsafe.Pointer(&b[0])
	}
	return unsafe.Pointer(&_zero)
}

func _getPtr(b []byte) uintptr {
	return uintptr(_getStartPtr(b))
}

func _getBytes(ptr uintptr, len int, cap int) []byte {
	var sl = reflect.SliceHeader{Data: ptr, Len: len, Cap: cap}
	return *(*[]byte)(unsafe.Pointer(&sl))
}
