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

package kcp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/sha1"
	"unsafe"

	xor "github.com/templexxx/xorsimd"
	"github.com/tjfoc/gmsm/sm4"

	"golang.org/x/crypto/blowfish"
	"golang.org/x/crypto/cast5"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/salsa20"
	"golang.org/x/crypto/tea"
	"golang.org/x/crypto/twofish"
	"golang.org/x/crypto/xtea"
)

var (
	// a defined initial vector
	// https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#Initialization_vector_.28IV.29
	// https://en.wikipedia.org/wiki/Initialization_vector
	// actually initial vector is not used in this package, we prepend a random nonce to each outgoing packets.
	// though IV is fixed, the first 8 bytes of the encrypted data is always random.
	initialVector = []byte{167, 115, 79, 156, 18, 172, 27, 1, 164, 21, 242, 193, 252, 120, 230, 107}
	saltxor       = `sH3CIVoF#rWLtJo6`
)

// BlockCrypt defines encryption/decryption methods for a given byte slice.
// Notes on implementing: the data to be encrypted contains a builtin
// nonce at the first 16 bytes
type BlockCrypt interface {
	// Encrypt encrypts the whole block in src into dst.
	// Dst and src may point at the same memory.
	Encrypt(dst, src []byte)

	// Decrypt decrypts the whole block in src into dst.
	// Dst and src may point at the same memory.
	Decrypt(dst, src []byte)
}

type salsa20BlockCrypt struct {
	key [32]byte
}

// NewSalsa20BlockCrypt https://en.wikipedia.org/wiki/Salsa20
func NewSalsa20BlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(salsa20BlockCrypt)
	copy(c.key[:], key)
	return c, nil
}

func (c *salsa20BlockCrypt) Encrypt(dst, src []byte) {
	salsa20.XORKeyStream(dst[8:], src[8:], src[:8], &c.key)
	copy(dst[:8], src[:8])
}
func (c *salsa20BlockCrypt) Decrypt(dst, src []byte) {
	salsa20.XORKeyStream(dst[8:], src[8:], src[:8], &c.key)
	copy(dst[:8], src[:8])
}

type sm4BlockCrypt struct {
	encbuf [sm4.BlockSize]byte // 64bit alignment enc/dec buffer
	decbuf [2 * sm4.BlockSize]byte
	block  cipher.Block
}

// NewSM4BlockCrypt https://github.com/tjfoc/gmsm/tree/master/sm4
func NewSM4BlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(sm4BlockCrypt)
	block, err := sm4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *sm4BlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *sm4BlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type twofishBlockCrypt struct {
	encbuf [twofish.BlockSize]byte
	decbuf [2 * twofish.BlockSize]byte
	block  cipher.Block
}

// NewTwofishBlockCrypt https://en.wikipedia.org/wiki/Twofish
func NewTwofishBlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(twofishBlockCrypt)
	block, err := twofish.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *twofishBlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *twofishBlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type tripleDESBlockCrypt struct {
	encbuf [des.BlockSize]byte
	decbuf [2 * des.BlockSize]byte
	block  cipher.Block
}

// NewTripleDESBlockCrypt https://en.wikipedia.org/wiki/Triple_DES
func NewTripleDESBlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(tripleDESBlockCrypt)
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *tripleDESBlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *tripleDESBlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type cast5BlockCrypt struct {
	encbuf [cast5.BlockSize]byte
	decbuf [2 * cast5.BlockSize]byte
	block  cipher.Block
}

// NewCast5BlockCrypt https://en.wikipedia.org/wiki/CAST-128
func NewCast5BlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(cast5BlockCrypt)
	block, err := cast5.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *cast5BlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *cast5BlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type blowfishBlockCrypt struct {
	encbuf [blowfish.BlockSize]byte
	decbuf [2 * blowfish.BlockSize]byte
	block  cipher.Block
}

// NewBlowfishBlockCrypt https://en.wikipedia.org/wiki/Blowfish_(cipher)
func NewBlowfishBlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(blowfishBlockCrypt)
	block, err := blowfish.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *blowfishBlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *blowfishBlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type aesBlockCrypt struct {
	encbuf [aes.BlockSize]byte
	decbuf [2 * aes.BlockSize]byte
	block  cipher.Block
}

// NewAESBlockCrypt https://en.wikipedia.org/wiki/Advanced_Encryption_Standard
func NewAESBlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(aesBlockCrypt)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *aesBlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *aesBlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type teaBlockCrypt struct {
	encbuf [tea.BlockSize]byte
	decbuf [2 * tea.BlockSize]byte
	block  cipher.Block
}

// NewTEABlockCrypt https://en.wikipedia.org/wiki/Tiny_Encryption_Algorithm
func NewTEABlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(teaBlockCrypt)
	block, err := tea.NewCipherWithRounds(key, 16)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *teaBlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *teaBlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type xteaBlockCrypt struct {
	encbuf [xtea.BlockSize]byte
	decbuf [2 * xtea.BlockSize]byte
	block  cipher.Block
}

// NewXTEABlockCrypt https://en.wikipedia.org/wiki/XTEA
func NewXTEABlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(xteaBlockCrypt)
	block, err := xtea.NewCipher(key)
	if err != nil {
		return nil, err
	}
	c.block = block
	return c, nil
}

func (c *xteaBlockCrypt) Encrypt(dst, src []byte) { encrypt(c.block, dst, src, c.encbuf[:]) }
func (c *xteaBlockCrypt) Decrypt(dst, src []byte) { decrypt(c.block, dst, src, c.decbuf[:]) }

type simpleXORBlockCrypt struct {
	xortbl []byte
}

// NewSimpleXORBlockCrypt simple xor with key expanding
func NewSimpleXORBlockCrypt(key []byte) (BlockCrypt, error) {
	c := new(simpleXORBlockCrypt)
	c.xortbl = pbkdf2.Key(key, []byte(saltxor), 32, mtuLimit, sha1.New)
	return c, nil
}

func (c *simpleXORBlockCrypt) Encrypt(dst, src []byte) { xor.Bytes(dst, src, c.xortbl) }
func (c *simpleXORBlockCrypt) Decrypt(dst, src []byte) { xor.Bytes(dst, src, c.xortbl) }

type noneBlockCrypt struct{}

// NewNoneBlockCrypt does nothing but copying
func NewNoneBlockCrypt(key []byte) (BlockCrypt, error) {
	return new(noneBlockCrypt), nil
}

func (c *noneBlockCrypt) Encrypt(dst, src []byte) { copy(dst, src) }
func (c *noneBlockCrypt) Decrypt(dst, src []byte) { copy(dst, src) }

// packet encryption with local CFB mode
func encrypt(block cipher.Block, dst, src, buf []byte) {
	switch block.BlockSize() {
	case 8:
		encrypt8(block, dst, src, buf)
	case 16:
		encrypt16(block, dst, src, buf)
	default:
		panic("unsupported cipher block size")
	}
}

// optimized encryption for the ciphers which works in 8-bytes
func encrypt8(block cipher.Block, dst, src, buf []byte) {
	tbl := buf[:8]
	block.Encrypt(tbl, initialVector)
	n := len(src) / 8
	base := 0
	repeat := n / 8
	left := n % 8
	ptr_tbl := (*uint64)(unsafe.Pointer(&tbl[0]))

	for i := 0; i < repeat; i++ {
		s := src[base:][0:64]
		d := dst[base:][0:64]
		// 1
		*(*uint64)(unsafe.Pointer(&d[0])) = *(*uint64)(unsafe.Pointer(&s[0])) ^ *ptr_tbl
		block.Encrypt(tbl, d[0:8])
		// 2
		*(*uint64)(unsafe.Pointer(&d[8])) = *(*uint64)(unsafe.Pointer(&s[8])) ^ *ptr_tbl
		block.Encrypt(tbl, d[8:16])
		// 3
		*(*uint64)(unsafe.Pointer(&d[16])) = *(*uint64)(unsafe.Pointer(&s[16])) ^ *ptr_tbl
		block.Encrypt(tbl, d[16:24])
		// 4
		*(*uint64)(unsafe.Pointer(&d[24])) = *(*uint64)(unsafe.Pointer(&s[24])) ^ *ptr_tbl
		block.Encrypt(tbl, d[24:32])
		// 5
		*(*uint64)(unsafe.Pointer(&d[32])) = *(*uint64)(unsafe.Pointer(&s[32])) ^ *ptr_tbl
		block.Encrypt(tbl, d[32:40])
		// 6
		*(*uint64)(unsafe.Pointer(&d[40])) = *(*uint64)(unsafe.Pointer(&s[40])) ^ *ptr_tbl
		block.Encrypt(tbl, d[40:48])
		// 7
		*(*uint64)(unsafe.Pointer(&d[48])) = *(*uint64)(unsafe.Pointer(&s[48])) ^ *ptr_tbl
		block.Encrypt(tbl, d[48:56])
		// 8
		*(*uint64)(unsafe.Pointer(&d[56])) = *(*uint64)(unsafe.Pointer(&s[56])) ^ *ptr_tbl
		block.Encrypt(tbl, d[56:64])
		base += 64
	}

	switch left {
	case 7:
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *ptr_tbl
		block.Encrypt(tbl, dst[base:])
		base += 8
		fallthrough
	case 6:
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *ptr_tbl
		block.Encrypt(tbl, dst[base:])
		base += 8
		fallthrough
	case 5:
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *ptr_tbl
		block.Encrypt(tbl, dst[base:])
		base += 8
		fallthrough
	case 4:
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *ptr_tbl
		block.Encrypt(tbl, dst[base:])
		base += 8
		fallthrough
	case 3:
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *ptr_tbl
		block.Encrypt(tbl, dst[base:])
		base += 8
		fallthrough
	case 2:
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *ptr_tbl
		block.Encrypt(tbl, dst[base:])
		base += 8
		fallthrough
	case 1:
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *ptr_tbl
		block.Encrypt(tbl, dst[base:])
		base += 8
		fallthrough
	case 0:
		xorBytes(dst[base:], src[base:], tbl)
	}
}

// optimized encryption for the ciphers which works in 16-bytes
func encrypt16(block cipher.Block, dst, src, buf []byte) {
	tbl := buf[:16]
	block.Encrypt(tbl, initialVector)
	n := len(src) / 16
	base := 0
	repeat := n / 8
	left := n % 8
	for i := 0; i < repeat; i++ {
		s := src[base:][0:128]
		d := dst[base:][0:128]
		// 1
		xor.Bytes16Align(d[0:16], s[0:16], tbl)
		block.Encrypt(tbl, d[0:16])
		// 2
		xor.Bytes16Align(d[16:32], s[16:32], tbl)
		block.Encrypt(tbl, d[16:32])
		// 3
		xor.Bytes16Align(d[32:48], s[32:48], tbl)
		block.Encrypt(tbl, d[32:48])
		// 4
		xor.Bytes16Align(d[48:64], s[48:64], tbl)
		block.Encrypt(tbl, d[48:64])
		// 5
		xor.Bytes16Align(d[64:80], s[64:80], tbl)
		block.Encrypt(tbl, d[64:80])
		// 6
		xor.Bytes16Align(d[80:96], s[80:96], tbl)
		block.Encrypt(tbl, d[80:96])
		// 7
		xor.Bytes16Align(d[96:112], s[96:112], tbl)
		block.Encrypt(tbl, d[96:112])
		// 8
		xor.Bytes16Align(d[112:128], s[112:128], tbl)
		block.Encrypt(tbl, d[112:128])
		base += 128
	}

	switch left {
	case 7:
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += 16
		fallthrough
	case 6:
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += 16
		fallthrough
	case 5:
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += 16
		fallthrough
	case 4:
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += 16
		fallthrough
	case 3:
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += 16
		fallthrough
	case 2:
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += 16
		fallthrough
	case 1:
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		block.Encrypt(tbl, dst[base:])
		base += 16
		fallthrough
	case 0:
		xorBytes(dst[base:], src[base:], tbl)
	}
}

// decryption
func decrypt(block cipher.Block, dst, src, buf []byte) {
	switch block.BlockSize() {
	case 8:
		decrypt8(block, dst, src, buf)
	case 16:
		decrypt16(block, dst, src, buf)
	default:
		panic("unsupported cipher block size")
	}
}

// decrypt 8 bytes block, all byte slices are supposed to be 64bit aligned
func decrypt8(block cipher.Block, dst, src, buf []byte) {
	tbl := buf[0:8]
	next := buf[8:16]
	block.Encrypt(tbl, initialVector)
	n := len(src) / 8
	base := 0
	repeat := n / 8
	left := n % 8
	ptr_tbl := (*uint64)(unsafe.Pointer(&tbl[0]))
	ptr_next := (*uint64)(unsafe.Pointer(&next[0]))

	// loop unrolling to relieve data dependency
	for i := 0; i < repeat; i++ {
		s := src[base:][0:64]
		d := dst[base:][0:64]
		// 1
		block.Encrypt(next, s[0:8])
		*(*uint64)(unsafe.Pointer(&d[0])) = *(*uint64)(unsafe.Pointer(&s[0])) ^ *ptr_tbl
		// 2
		block.Encrypt(tbl, s[8:16])
		*(*uint64)(unsafe.Pointer(&d[8])) = *(*uint64)(unsafe.Pointer(&s[8])) ^ *ptr_next
		// 3
		block.Encrypt(next, s[16:24])
		*(*uint64)(unsafe.Pointer(&d[16])) = *(*uint64)(unsafe.Pointer(&s[16])) ^ *ptr_tbl
		// 4
		block.Encrypt(tbl, s[24:32])
		*(*uint64)(unsafe.Pointer(&d[24])) = *(*uint64)(unsafe.Pointer(&s[24])) ^ *ptr_next
		// 5
		block.Encrypt(next, s[32:40])
		*(*uint64)(unsafe.Pointer(&d[32])) = *(*uint64)(unsafe.Pointer(&s[32])) ^ *ptr_tbl
		// 6
		block.Encrypt(tbl, s[40:48])
		*(*uint64)(unsafe.Pointer(&d[40])) = *(*uint64)(unsafe.Pointer(&s[40])) ^ *ptr_next
		// 7
		block.Encrypt(next, s[48:56])
		*(*uint64)(unsafe.Pointer(&d[48])) = *(*uint64)(unsafe.Pointer(&s[48])) ^ *ptr_tbl
		// 8
		block.Encrypt(tbl, s[56:64])
		*(*uint64)(unsafe.Pointer(&d[56])) = *(*uint64)(unsafe.Pointer(&s[56])) ^ *ptr_next
		base += 64
	}

	switch left {
	case 7:
		block.Encrypt(next, src[base:])
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *(*uint64)(unsafe.Pointer(&tbl[0]))
		tbl, next = next, tbl
		base += 8
		fallthrough
	case 6:
		block.Encrypt(next, src[base:])
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *(*uint64)(unsafe.Pointer(&tbl[0]))
		tbl, next = next, tbl
		base += 8
		fallthrough
	case 5:
		block.Encrypt(next, src[base:])
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *(*uint64)(unsafe.Pointer(&tbl[0]))
		tbl, next = next, tbl
		base += 8
		fallthrough
	case 4:
		block.Encrypt(next, src[base:])
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *(*uint64)(unsafe.Pointer(&tbl[0]))
		tbl, next = next, tbl
		base += 8
		fallthrough
	case 3:
		block.Encrypt(next, src[base:])
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *(*uint64)(unsafe.Pointer(&tbl[0]))
		tbl, next = next, tbl
		base += 8
		fallthrough
	case 2:
		block.Encrypt(next, src[base:])
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *(*uint64)(unsafe.Pointer(&tbl[0]))
		tbl, next = next, tbl
		base += 8
		fallthrough
	case 1:
		block.Encrypt(next, src[base:])
		*(*uint64)(unsafe.Pointer(&dst[base])) = *(*uint64)(unsafe.Pointer(&src[base])) ^ *(*uint64)(unsafe.Pointer(&tbl[0]))
		tbl, next = next, tbl
		base += 8
		fallthrough
	case 0:
		xorBytes(dst[base:], src[base:], tbl)
	}
}

func decrypt16(block cipher.Block, dst, src, buf []byte) {
	tbl := buf[0:16]
	next := buf[16:32]
	block.Encrypt(tbl, initialVector)
	n := len(src) / 16
	base := 0
	repeat := n / 8
	left := n % 8

	// loop unrolling to relieve data dependency
	for i := 0; i < repeat; i++ {
		s := src[base:][0:128]
		d := dst[base:][0:128]
		// 1
		block.Encrypt(next, s[0:16])
		xor.Bytes16Align(d[0:16], s[0:16], tbl)
		// 2
		block.Encrypt(tbl, s[16:32])
		xor.Bytes16Align(d[16:32], s[16:32], next)
		// 3
		block.Encrypt(next, s[32:48])
		xor.Bytes16Align(d[32:48], s[32:48], tbl)
		// 4
		block.Encrypt(tbl, s[48:64])
		xor.Bytes16Align(d[48:64], s[48:64], next)
		// 5
		block.Encrypt(next, s[64:80])
		xor.Bytes16Align(d[64:80], s[64:80], tbl)
		// 6
		block.Encrypt(tbl, s[80:96])
		xor.Bytes16Align(d[80:96], s[80:96], next)
		// 7
		block.Encrypt(next, s[96:112])
		xor.Bytes16Align(d[96:112], s[96:112], tbl)
		// 8
		block.Encrypt(tbl, s[112:128])
		xor.Bytes16Align(d[112:128], s[112:128], next)
		base += 128
	}

	switch left {
	case 7:
		block.Encrypt(next, src[base:])
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += 16
		fallthrough
	case 6:
		block.Encrypt(next, src[base:])
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += 16
		fallthrough
	case 5:
		block.Encrypt(next, src[base:])
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += 16
		fallthrough
	case 4:
		block.Encrypt(next, src[base:])
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += 16
		fallthrough
	case 3:
		block.Encrypt(next, src[base:])
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += 16
		fallthrough
	case 2:
		block.Encrypt(next, src[base:])
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += 16
		fallthrough
	case 1:
		block.Encrypt(next, src[base:])
		xor.Bytes16Align(dst[base:], src[base:], tbl)
		tbl, next = next, tbl
		base += 16
		fallthrough
	case 0:
		xorBytes(dst[base:], src[base:], tbl)
	}
}

// per bytes xors
func xorBytes(dst, a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0
	}

	for i := 0; i < n; i++ {
		dst[i] = a[i] ^ b[i]
	}
	return n
}
