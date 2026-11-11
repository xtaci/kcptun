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

import "github.com/awnumar/memguard"

// ProtectedKey keeps a pre-shared key in read-only, locked memory.
// Call Destroy when the key is no longer needed.
type ProtectedKey struct {
	buffer *memguard.LockedBuffer
	size   int
}

// NewProtectedKey copies key into locked memory and makes it read-only.
// The original string and any other copies are not protected or wiped.
func NewProtectedKey(key string) *ProtectedKey {
	// memguard requires a nonzero allocation, even for an empty key.
	buffer := memguard.NewBuffer(max(1, len(key)))
	copy(buffer.Bytes(), key)
	buffer.Freeze()
	return &ProtectedKey{buffer: buffer, size: len(key)}
}

// Bytes returns a read-only view of the key, valid until Destroy is called.
// The returned slice must not be modified or used concurrently with Destroy.
func (k *ProtectedKey) Bytes() []byte {
	return k.buffer.Bytes()[:k.size]
}

// Destroy wipes and releases the locked buffer. It is safe to call more than once.
func (k *ProtectedKey) Destroy() {
	k.buffer.Destroy()
}
