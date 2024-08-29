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
	"crypto/md5"
	"crypto/rand"
	"io"
)

// Entropy defines a entropy source
//
//	Entropy in this file generate random bits for the first 16-bytes of each packets.
type Entropy interface {
	Init()
	Fill(nonce []byte)
}

// nonceMD5 nonce generator for packet header
type nonceMD5 struct {
	seed [md5.Size]byte
}

func (n *nonceMD5) Init() { /*nothing required*/ }

func (n *nonceMD5) Fill(nonce []byte) {
	if n.seed[0] == 0 { // entropy update
		io.ReadFull(rand.Reader, n.seed[:])
	}
	n.seed = md5.Sum(n.seed[:])
	copy(nonce, n.seed[:])
}

// nonceAES128 nonce generator for packet headers
type nonceAES128 struct {
	seed  [aes.BlockSize]byte
	block cipher.Block
}

func (n *nonceAES128) Init() {
	var key [16]byte //aes-128
	io.ReadFull(rand.Reader, key[:])
	io.ReadFull(rand.Reader, n.seed[:])
	block, _ := aes.NewCipher(key[:])
	n.block = block
}

func (n *nonceAES128) Fill(nonce []byte) {
	if n.seed[0] == 0 { // entropy update
		io.ReadFull(rand.Reader, n.seed[:])
	}
	n.block.Encrypt(n.seed[:], n.seed[:])
	copy(nonce, n.seed[:])
}
