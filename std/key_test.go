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

import (
	"bytes"
	"crypto/cipher"
	"crypto/sha1"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func TestProtectedKey(t *testing.T) {
	for _, value := range []string{"", "secret", "密钥", string(bytes.Repeat([]byte("x"), 8193))} {
		t.Run(value[:min(len(value), 10)], func(t *testing.T) {
			key := NewProtectedKey(value)
			if !key.buffer.Alive() || key.buffer.Mutable() {
				t.Fatal("key buffer must be alive and read-only")
			}
			if !bytes.Equal(key.Bytes(), []byte(value)) {
				t.Fatal("protected key differs from input")
			}
			key.Destroy()
			if key.buffer.Alive() {
				t.Fatal("key buffer was not destroyed")
			}
			key.Destroy()
		})
	}
}

func TestDeriveBlockCryptCompatibility(t *testing.T) {
	for _, secret := range []string{"", "secret", "密钥"} {
		for _, method := range []string{"aes", "aes-128", "aes-192", "aes-128-gcm", "salsa20", "xor", "none", "null", "sm4", "tea", "blowfish", "twofish", "cast5", "3des", "xtea", "unknown"} {
			t.Run(secret+"/"+method, func(t *testing.T) {
				key := NewProtectedKey(secret)
				protected, actualMethod := DeriveBlockCrypt(method, key.Bytes(), "kcp-go")
				key.Destroy()
				pass := pbkdf2.Key([]byte(secret), []byte("kcp-go"), 4096, 32, sha1.New)
				legacy, expectedMethod := SelectBlockCrypt(method, pass)
				clear(pass)
				if actualMethod != expectedMethod || (protected == nil) != (legacy == nil) {
					t.Fatal("cipher selection differs from legacy derivation")
				}
				if protected == nil {
					return
				}
				plaintext := bytes.Repeat([]byte("test data"), 8)
				if aead, ok := protected.(cipher.AEAD); ok {
					other := legacy.(cipher.AEAD)
					nonce := make([]byte, aead.NonceSize())
					ciphertext := aead.Seal(make([]byte, 0, len(plaintext)+aead.Overhead()), nonce, plaintext, nil)
					decoded, err := other.Open(nil, nonce, ciphertext, nil)
					if err != nil || !bytes.Equal(decoded, plaintext) {
						t.Fatalf("AEAD compatibility failed: %v", err)
					}
					return
				}
				ciphertext := make([]byte, len(plaintext))
				decoded := make([]byte, len(plaintext))
				protected.Encrypt(ciphertext, plaintext)
				legacy.Decrypt(decoded, ciphertext)
				if !bytes.Equal(decoded, plaintext) {
					t.Fatal("ciphertext is incompatible with legacy derivation")
				}
			})
		}
	}
}
