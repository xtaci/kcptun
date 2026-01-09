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
	kcp "github.com/xtaci/kcp-go/v5"
)

// SelectBlockCrypt translates a human readable cipher name into the concrete
// kcp.BlockCrypt implementation. It also reports the effective cipher name after
// applying fallbacks so callers can log the final choice.
func SelectBlockCrypt(method string, pass []byte) (kcp.BlockCrypt, string) {
	switch method {
	case "null":
		return nil, method
	case "sm4":
		block, _ := kcp.NewSM4BlockCrypt(pass[:16])
		return block, method
	case "tea":
		block, _ := kcp.NewTEABlockCrypt(pass[:16])
		return block, method
	case "xor":
		block, _ := kcp.NewSimpleXORBlockCrypt(pass)
		return block, method
	case "none":
		block, _ := kcp.NewNoneBlockCrypt(pass)
		return block, method
	case "aes-128":
		block, _ := kcp.NewAESBlockCrypt(pass[:16])
		return block, method
	case "aes-192":
		block, _ := kcp.NewAESBlockCrypt(pass[:24])
		return block, method
	case "blowfish":
		block, _ := kcp.NewBlowfishBlockCrypt(pass)
		return block, method
	case "twofish":
		block, _ := kcp.NewTwofishBlockCrypt(pass)
		return block, method
	case "cast5":
		block, _ := kcp.NewCast5BlockCrypt(pass[:16])
		return block, method
	case "3des":
		block, _ := kcp.NewTripleDESBlockCrypt(pass[:24])
		return block, method
	case "xtea":
		block, _ := kcp.NewXTEABlockCrypt(pass[:16])
		return block, method
	case "salsa20":
		block, _ := kcp.NewSalsa20BlockCrypt(pass)
		return block, method
	case "aes-128-gcm":
		block, _ := kcp.NewAESGCMCrypt(pass[:16])
		return block, method
	default:
		block, _ := kcp.NewAESBlockCrypt(pass)
		return block, "aes"
	}
}
