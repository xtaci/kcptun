// The MIT License (MIT)
//
// # Copyright (c) 2016 xtaci
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

import (
	"log"

	kcp "github.com/xtaci/kcp-go/v5"
)

// cryptMethod maps cipher names to their constructor functions and required key sizes.
type cryptMethod struct {
	keySize int // required key size (0 means use full key)
	build   func(key []byte) (kcp.BlockCrypt, error)
}

// cryptMethods is a lookup table for supported encryption methods.
// Using a map simplifies the code and makes adding new ciphers easier.
var cryptMethods = map[string]cryptMethod{
	"null":        {0, func(key []byte) (kcp.BlockCrypt, error) { return nil, nil }},
	"sm4":         {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewSM4BlockCrypt(key) }},
	"tea":         {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewTEABlockCrypt(key) }},
	"xor":         {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewSimpleXORBlockCrypt(key) }},
	"none":        {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewNoneBlockCrypt(key) }},
	"aes-128":     {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewAESBlockCrypt(key) }},
	"aes-192":     {24, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewAESBlockCrypt(key) }},
	"blowfish":    {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewBlowfishBlockCrypt(key) }},
	"twofish":     {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewTwofishBlockCrypt(key) }},
	"cast5":       {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewCast5BlockCrypt(key) }},
	"3des":        {24, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewTripleDESBlockCrypt(key) }},
	"xtea":        {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewXTEABlockCrypt(key) }},
	"salsa20":     {0, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewSalsa20BlockCrypt(key) }},
	"aes-128-gcm": {16, func(key []byte) (kcp.BlockCrypt, error) { return kcp.NewAESGCMCrypt(key) }},
}

// SelectBlockCrypt translates a human readable cipher name into the concrete
// kcp.BlockCrypt implementation. It also reports the effective cipher name after
// applying fallbacks so callers can log the final choice.
func SelectBlockCrypt(method string, pass []byte) (kcp.BlockCrypt, string) {
	if m, ok := cryptMethods[method]; ok {
		key := pass
		if m.keySize > 0 && len(pass) >= m.keySize {
			key = pass[:m.keySize]
		}
		block, err := m.build(key)
		if err != nil {
			log.Printf("crypt: failed to create %s cipher: %v, falling back to aes", method, err)
			block, _ = kcp.NewAESBlockCrypt(pass)
			return block, "aes"
		}
		return block, method
	}
	// Default to AES for unknown methods
	block, err := kcp.NewAESBlockCrypt(pass)
	if err != nil {
		log.Printf("crypt: failed to create default aes cipher: %v", err)
	}
	return block, "aes"
}
