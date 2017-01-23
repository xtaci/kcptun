// # Copyright (c) 2024 xtaci
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package qpp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/big"
	"unsafe"

	"golang.org/x/crypto/pbkdf2"
)

// Constants used in Quantum Permutation Pad (QPP) for identifiers, salts, and configuration
const (
	PAD_IDENTIFIER         = "QPP_%b"
	PM_SELECTOR_IDENTIFIER = "PERMUTATION_MATRIX_SELECTOR"
	SHUFFLE_SALT           = "___QUANTUM_PERMUTATION_PAD_SHUFFLE_SALT___"
	PRNG_SALT              = "___QUANTUM_PERMUTATION_PAD_PRNG_SALT___"
	NATIVE_BYTE_LENGTH     = 8   // Bit length for native byte
	PBKDF2_LOOPS           = 128 // Number of iterations for PBKDF2
	CHUNK_DERIVE_SALT      = "___QUANTUM_PERMUTATION_PAD_SEED_DERIVE___"
	CHUNK_DERIVE_LOOPS     = 1024
	PAD_SWITCH             = 8 // switch pad for every PAD_SWITCH bytes
	QUBITS                 = 8 // number of quantum bits of this implementation
)

// Rand is a stateful random number generator
// Fields are ordered for optimal cache line usage (64-byte cache line on modern CPUs)
type Rand struct {
	xoshiro [4]uint64 // xoshiro state (32 bytes)
	seed64  uint64    // the latest random number (8 bytes)
	count   uint8     // number of bytes encrypted, counted in modular arithmetic (1 byte)
	_       [7]byte   // padding to align to 48 bytes total
}

// QuantumPermutationPad represents the encryption/decryption structure using quantum permutation pads
// QPP is a cryptographic technique that leverages quantum-inspired permutation matrices to provide secure encryption.
// Fields are ordered for optimal cache line usage - hot path data first.
type QuantumPermutationPad struct {
	// Hot path fields - accessed every encryption/decryption call
	padsPtr  unsafe.Pointer // raw pointer to encryption pads
	rpadsPtr unsafe.Pointer // raw pointer to decryption pads
	numPads  uint16         // Number of pads (permutation matrices)

	encRand *Rand // Default random source for encryption pad selection
	decRand *Rand // Default random source for decryption pad selection

	// Cold path - only used for reference
	pads  []byte // Encryption pads, each pad is a permutation matrix for encryption
	rpads []byte // Decryption pads, each pad is a reverse permutation matrix for decryption
}

// NewQPP creates a new Quantum Permutation Pad instance with the provided seed, number of pads, and qubits
// The seed is used to generate deterministic pseudo-random number generators (PRNGs) for both encryption and decryption
func NewQPP(seed []byte, numPads uint16) *QuantumPermutationPad {
	qpp := &QuantumPermutationPad{
		numPads: numPads,
	}

	matrixBytes := 1 << QUBITS
	qpp.pads = make([]byte, int(numPads)*matrixBytes)
	qpp.rpads = make([]byte, int(numPads)*matrixBytes)
	qpp.padsPtr = unsafe.Pointer(unsafe.SliceData(qpp.pads))
	qpp.rpadsPtr = unsafe.Pointer(unsafe.SliceData(qpp.rpads))

	chunks := seedToChunks(seed, QUBITS)
	// creat AES-256 blocks to generate random number for shuffling
	var blocks []cipher.Block
	for _, chunk := range chunks {
		aeskey := pbkdf2.Key(chunk, []byte(SHUFFLE_SALT), PBKDF2_LOOPS, 32, sha1.New)
		block, err := aes.NewCipher(aeskey)
		if err != nil {
			panic(fmt.Sprintf("NewQPP: failed to create AES cipher block: %v", err))
		}
		blocks = append(blocks, block)
	}

	// Initialize and shuffle pads to create permutation matrices
	for i := range int(numPads) {
		pad := qpp.pads[i*matrixBytes : (i+1)*matrixBytes]
		rpad := qpp.rpads[i*matrixBytes : (i+1)*matrixBytes]

		// Fill pad with sequential byte values
		fill(pad)
		// Shuffle pad to create a unique permutation matrix
		shuffle(chunks[i%len(chunks)], pad, uint16(i), blocks)
		// Create the reverse permutation matrix for decryption
		reverse(pad, rpad)
	}

	qpp.encRand = CreatePRNG(seed) // Create default PRNG for encryption
	qpp.decRand = CreatePRNG(seed) // Create default PRNG for decryption

	return qpp
}

// Encrypt encrypts the given data using the Quantum Permutation Pad with the default PRNG
// It selects a permutation matrix based on a random index and uses it to permute each byte of the data
func (qpp *QuantumPermutationPad) Encrypt(data []byte) {
	qpp.EncryptWithPRNG(data, qpp.encRand)
}

// Decrypt decrypts the given data using the Quantum Permutation Pad with the default PRNG
// It selects a reverse permutation matrix based on a random index and uses it to restore each byte of the data
func (qpp *QuantumPermutationPad) Decrypt(data []byte) {
	qpp.DecryptWithPRNG(data, qpp.decRand)
}

// CreatePRNG creates a deterministic pseudo-random number generator based on the provided seed
// It uses HMAC and PBKDF2 to derive a random seed for the PRNG
func CreatePRNG(seed []byte) *Rand {
	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte(PM_SELECTOR_IDENTIFIER))
	sum := mac.Sum(nil)

	// Derive a key for xoroshiro256**
	xoshiro := pbkdf2.Key(sum, []byte(PRNG_SALT), PBKDF2_LOOPS, 32, sha1.New)
	// Create and return PRNG
	rd := &Rand{}
	rd.xoshiro[0] = binary.LittleEndian.Uint64(xoshiro[0:8])
	rd.xoshiro[1] = binary.LittleEndian.Uint64(xoshiro[8:16])
	rd.xoshiro[2] = binary.LittleEndian.Uint64(xoshiro[16:24])
	rd.xoshiro[3] = binary.LittleEndian.Uint64(xoshiro[24:32])
	rd.seed64 = xoshiro256ss(&rd.xoshiro)
	return rd
}

// FastPRNG creates a deterministic pseudo-random number generator based on the provided seed, but with a faster initialization,
// it's suitable for the cases where the seed have sufficient randomness.
func FastPRNG(seed []byte) *Rand {
	sum := sha256.Sum256(seed)

	// Create and return PRNG
	rd := &Rand{}
	rd.xoshiro[0] = binary.LittleEndian.Uint64(sum[0:8])
	rd.xoshiro[1] = binary.LittleEndian.Uint64(sum[8:16])
	rd.xoshiro[2] = binary.LittleEndian.Uint64(sum[16:24])
	rd.xoshiro[3] = binary.LittleEndian.Uint64(sum[24:32])
	rd.seed64 = xoshiro256ss(&rd.xoshiro)
	return rd
}

// EncryptWithPRNG encrypts the data using the Quantum Permutation Pad with a custom PRNG
// The PRNG exposes 64-bit chunks; the `count` field tracks how many bytes of the
// current 64-bit word have already been consumed so that successive calls remain
// byte-aligned even if the caller streams arbitrary lengths.
//
//go:nosplit
func (qpp *QuantumPermutationPad) EncryptWithPRNG(data []byte, rand *Rand) {
	if len(data) == 0 {
		return
	}

	// initial r, index, count
	size := len(data)
	r := rand.seed64
	numPads := qpp.numPads
	padsPtr := qpp.padsPtr
	base := unsafe.Add(padsPtr, uintptr(uint16(r)%numPads)<<8)
	count := rand.count

	// inline xoshiro state for speed
	s0, s1, s2, s3 := rand.xoshiro[0], rand.xoshiro[1], rand.xoshiro[2], rand.xoshiro[3]

	// handle unaligned 8bytes
	if count != 0 {
		offset := 0
		for ; offset < len(data); offset++ {
			// Use the already generated 64-bit random word and keep consuming it byte by byte.
			rr := byte(r >> (count << 3))
			data[offset] = *(*byte)(unsafe.Add(base, uintptr(data[offset]^rr)))
			count++

			// switch to another pad when count reaches PAD_SWITCH
			if count == PAD_SWITCH {
				// inline xoshiro256**
				r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
				t := s1 << 17
				s2 ^= s0
				s3 ^= s1
				s1 ^= s2
				s0 ^= s3
				s2 ^= t
				s3 = (s3 << 45) | (s3 >> 19)

				base = unsafe.Add(padsPtr, uintptr(uint16(r)%numPads)<<8)
				offset++
				count = 0
				break
			}
		}
		data = data[offset:] // aligned bytes start from here
	}

	// handle 8-byte aligned blocks with 2x unrolling for better ILP
	repeat := len(data) >> 4 // process 16 bytes at a time
	for i := 0; i < repeat; i++ {
		d := data[i<<4:]
		_ = d[15] // bounds check elimination

		// first 8 bytes
		x0 := uintptr(d[0] ^ byte(r))
		x1 := uintptr(d[1] ^ byte(r>>8))
		x2 := uintptr(d[2] ^ byte(r>>16))
		x3 := uintptr(d[3] ^ byte(r>>24))
		x4 := uintptr(d[4] ^ byte(r>>32))
		x5 := uintptr(d[5] ^ byte(r>>40))
		x6 := uintptr(d[6] ^ byte(r>>48))
		x7 := uintptr(d[7] ^ byte(r>>56))

		d[0] = *(*byte)(unsafe.Add(base, x0))
		d[1] = *(*byte)(unsafe.Add(base, x1))
		d[2] = *(*byte)(unsafe.Add(base, x2))
		d[3] = *(*byte)(unsafe.Add(base, x3))
		d[4] = *(*byte)(unsafe.Add(base, x4))
		d[5] = *(*byte)(unsafe.Add(base, x5))
		d[6] = *(*byte)(unsafe.Add(base, x6))
		d[7] = *(*byte)(unsafe.Add(base, x7))

		// inline xoshiro256** for next 8 bytes
		r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
		t := s1 << 17
		s2 ^= s0
		s3 ^= s1
		s1 ^= s2
		s0 ^= s3
		s2 ^= t
		s3 = (s3 << 45) | (s3 >> 19)
		base = unsafe.Add(padsPtr, uintptr(uint16(r)%numPads)<<8)

		// second 8 bytes
		x0 = uintptr(d[8] ^ byte(r))
		x1 = uintptr(d[9] ^ byte(r>>8))
		x2 = uintptr(d[10] ^ byte(r>>16))
		x3 = uintptr(d[11] ^ byte(r>>24))
		x4 = uintptr(d[12] ^ byte(r>>32))
		x5 = uintptr(d[13] ^ byte(r>>40))
		x6 = uintptr(d[14] ^ byte(r>>48))
		x7 = uintptr(d[15] ^ byte(r>>56))

		d[8] = *(*byte)(unsafe.Add(base, x0))
		d[9] = *(*byte)(unsafe.Add(base, x1))
		d[10] = *(*byte)(unsafe.Add(base, x2))
		d[11] = *(*byte)(unsafe.Add(base, x3))
		d[12] = *(*byte)(unsafe.Add(base, x4))
		d[13] = *(*byte)(unsafe.Add(base, x5))
		d[14] = *(*byte)(unsafe.Add(base, x6))
		d[15] = *(*byte)(unsafe.Add(base, x7))

		// inline xoshiro256** for next iteration
		r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
		t = s1 << 17
		s2 ^= s0
		s3 ^= s1
		s1 ^= s2
		s0 ^= s3
		s2 ^= t
		s3 = (s3 << 45) | (s3 >> 19)
		base = unsafe.Add(padsPtr, uintptr(uint16(r)%numPads)<<8)
	}
	data = data[repeat<<4:]

	// handle remaining 8-byte block if any
	if len(data) >= 8 {
		d := data
		_ = d[7] // bounds check elimination

		x0 := uintptr(d[0] ^ byte(r))
		x1 := uintptr(d[1] ^ byte(r>>8))
		x2 := uintptr(d[2] ^ byte(r>>16))
		x3 := uintptr(d[3] ^ byte(r>>24))
		x4 := uintptr(d[4] ^ byte(r>>32))
		x5 := uintptr(d[5] ^ byte(r>>40))
		x6 := uintptr(d[6] ^ byte(r>>48))
		x7 := uintptr(d[7] ^ byte(r>>56))

		d[0] = *(*byte)(unsafe.Add(base, x0))
		d[1] = *(*byte)(unsafe.Add(base, x1))
		d[2] = *(*byte)(unsafe.Add(base, x2))
		d[3] = *(*byte)(unsafe.Add(base, x3))
		d[4] = *(*byte)(unsafe.Add(base, x4))
		d[5] = *(*byte)(unsafe.Add(base, x5))
		d[6] = *(*byte)(unsafe.Add(base, x6))
		d[7] = *(*byte)(unsafe.Add(base, x7))

		r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
		t := s1 << 17
		s2 ^= s0
		s3 ^= s1
		s1 ^= s2
		s0 ^= s3
		s2 ^= t
		s3 = (s3 << 45) | (s3 >> 19)
		base = unsafe.Add(padsPtr, uintptr(uint16(r)%numPads)<<8)
		data = data[8:]
	}

	// handle remaining tail bytes after the unrolled blocks
	for i := 0; i < len(data); i++ {
		rr := byte(r >> (count << 3))
		data[i] = *(*byte)(unsafe.Add(base, uintptr(data[i]^rr)))
		count++
	}

	// write back xoshiro state
	rand.xoshiro[0], rand.xoshiro[1], rand.xoshiro[2], rand.xoshiro[3] = s0, s1, s2, s3
	rand.seed64 = r
	rand.count = uint8((int(rand.count) + size) & (PAD_SWITCH - 1))
}

// DecryptWithPRNG mirrors EncryptWithPRNG but walks the reverse permutation pads so that
// the cipher stream remains synchronized with the same PRNG state.
//
//go:nosplit
func (qpp *QuantumPermutationPad) DecryptWithPRNG(data []byte, rand *Rand) {
	if len(data) == 0 {
		return
	}

	size := len(data)
	r := rand.seed64
	numPads := qpp.numPads
	rpadsPtr := qpp.rpadsPtr
	base := unsafe.Add(rpadsPtr, uintptr(uint16(r)%numPads)<<8)
	count := rand.count

	// inline xoshiro state for speed
	s0, s1, s2, s3 := rand.xoshiro[0], rand.xoshiro[1], rand.xoshiro[2], rand.xoshiro[3]

	// handle unaligned 8bytes
	if count != 0 {
		offset := 0
		for ; offset < len(data); offset++ {
			rr := byte(r >> (count << 3))
			data[offset] = *(*byte)(unsafe.Add(base, uintptr(data[offset]))) ^ rr
			count++

			if count == PAD_SWITCH {
				// inline xoshiro256**
				r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
				t := s1 << 17
				s2 ^= s0
				s3 ^= s1
				s1 ^= s2
				s0 ^= s3
				s2 ^= t
				s3 = (s3 << 45) | (s3 >> 19)

				base = unsafe.Add(rpadsPtr, uintptr(uint16(r)%numPads)<<8)
				offset++
				count = 0
				break
			}
		}
		data = data[offset:]
	}

	// handle 8-byte aligned blocks with 2x unrolling for better ILP
	repeat := len(data) >> 4 // process 16 bytes at a time
	for i := 0; i < repeat; i++ {
		d := data[i<<4:]
		_ = d[15] // bounds check elimination

		// first 8 bytes
		rr0, rr1 := byte(r), byte(r>>8)
		rr2, rr3 := byte(r>>16), byte(r>>24)
		rr4, rr5 := byte(r>>32), byte(r>>40)
		rr6, rr7 := byte(r>>48), byte(r>>56)

		d[0] = *(*byte)(unsafe.Add(base, uintptr(d[0]))) ^ rr0
		d[1] = *(*byte)(unsafe.Add(base, uintptr(d[1]))) ^ rr1
		d[2] = *(*byte)(unsafe.Add(base, uintptr(d[2]))) ^ rr2
		d[3] = *(*byte)(unsafe.Add(base, uintptr(d[3]))) ^ rr3
		d[4] = *(*byte)(unsafe.Add(base, uintptr(d[4]))) ^ rr4
		d[5] = *(*byte)(unsafe.Add(base, uintptr(d[5]))) ^ rr5
		d[6] = *(*byte)(unsafe.Add(base, uintptr(d[6]))) ^ rr6
		d[7] = *(*byte)(unsafe.Add(base, uintptr(d[7]))) ^ rr7

		// inline xoshiro256** for next 8 bytes
		r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
		t := s1 << 17
		s2 ^= s0
		s3 ^= s1
		s1 ^= s2
		s0 ^= s3
		s2 ^= t
		s3 = (s3 << 45) | (s3 >> 19)
		base = unsafe.Add(rpadsPtr, uintptr(uint16(r)%numPads)<<8)

		// second 8 bytes
		rr0, rr1 = byte(r), byte(r>>8)
		rr2, rr3 = byte(r>>16), byte(r>>24)
		rr4, rr5 = byte(r>>32), byte(r>>40)
		rr6, rr7 = byte(r>>48), byte(r>>56)

		d[8] = *(*byte)(unsafe.Add(base, uintptr(d[8]))) ^ rr0
		d[9] = *(*byte)(unsafe.Add(base, uintptr(d[9]))) ^ rr1
		d[10] = *(*byte)(unsafe.Add(base, uintptr(d[10]))) ^ rr2
		d[11] = *(*byte)(unsafe.Add(base, uintptr(d[11]))) ^ rr3
		d[12] = *(*byte)(unsafe.Add(base, uintptr(d[12]))) ^ rr4
		d[13] = *(*byte)(unsafe.Add(base, uintptr(d[13]))) ^ rr5
		d[14] = *(*byte)(unsafe.Add(base, uintptr(d[14]))) ^ rr6
		d[15] = *(*byte)(unsafe.Add(base, uintptr(d[15]))) ^ rr7

		// inline xoshiro256** for next iteration
		r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
		t = s1 << 17
		s2 ^= s0
		s3 ^= s1
		s1 ^= s2
		s0 ^= s3
		s2 ^= t
		s3 = (s3 << 45) | (s3 >> 19)
		base = unsafe.Add(rpadsPtr, uintptr(uint16(r)%numPads)<<8)
	}
	data = data[repeat<<4:]

	// handle remaining 8-byte block if any
	if len(data) >= 8 {
		d := data
		_ = d[7] // bounds check elimination

		rr0, rr1 := byte(r), byte(r>>8)
		rr2, rr3 := byte(r>>16), byte(r>>24)
		rr4, rr5 := byte(r>>32), byte(r>>40)
		rr6, rr7 := byte(r>>48), byte(r>>56)

		d[0] = *(*byte)(unsafe.Add(base, uintptr(d[0]))) ^ rr0
		d[1] = *(*byte)(unsafe.Add(base, uintptr(d[1]))) ^ rr1
		d[2] = *(*byte)(unsafe.Add(base, uintptr(d[2]))) ^ rr2
		d[3] = *(*byte)(unsafe.Add(base, uintptr(d[3]))) ^ rr3
		d[4] = *(*byte)(unsafe.Add(base, uintptr(d[4]))) ^ rr4
		d[5] = *(*byte)(unsafe.Add(base, uintptr(d[5]))) ^ rr5
		d[6] = *(*byte)(unsafe.Add(base, uintptr(d[6]))) ^ rr6
		d[7] = *(*byte)(unsafe.Add(base, uintptr(d[7]))) ^ rr7

		r = ((s1 * 5 << 7) | (s1 * 5 >> 57)) * 9
		t := s1 << 17
		s2 ^= s0
		s3 ^= s1
		s1 ^= s2
		s0 ^= s3
		s2 ^= t
		s3 = (s3 << 45) | (s3 >> 19)
		base = unsafe.Add(rpadsPtr, uintptr(uint16(r)%numPads)<<8)
		data = data[8:]
	}

	// handle remaining tail bytes; at this point `count` already encodes how many bytes of `r`
	// were consumed so the PRNG state stays identical to the encryption side.
	for i := 0; i < len(data); i++ {
		rr := byte(r >> (count << 3))
		data[i] = *(*byte)(unsafe.Add(base, uintptr(data[i]))) ^ rr
		count++
	}

	// write back xoshiro state
	rand.xoshiro[0], rand.xoshiro[1], rand.xoshiro[2], rand.xoshiro[3] = s0, s1, s2, s3
	rand.seed64 = r
	rand.count = uint8((int(rand.count) + size) & (PAD_SWITCH - 1))
}

// QPPMinimumSeedLength calculates the length required for the seed based on the number of qubits
// This ensures that the seed has sufficient entropy for the required permutations
func QPPMinimumSeedLength(qubits uint8) int {
	perms := big.NewInt(1 << qubits)
	for i := 1<<qubits - 1; i > 0; i-- {
		perms.Mul(perms, big.NewInt(int64(i)))
	}
	bitLen := perms.BitLen()
	byteLen := (bitLen + 7) / 8
	if byteLen == 0 {
		byteLen = 1
	}
	return byteLen
}

// QPPMinimumPads calculates the minimum number of pads required based on the number of qubits
// This is derived from the minimum seed length needed for the permutations
func QPPMinimumPads(qubits uint8) int {
	byteLen := QPPMinimumSeedLength(qubits)
	minpads := byteLen / 32
	left := byteLen % 32
	if left > 0 {
		minpads += 1
	}

	return minpads
}

// fill initializes the pad with sequential byte values
// This sets up a standard permutation matrix before it is shuffled
func fill(pad []byte) {
	pad[0] = 0
	for i := 1; i < len(pad); i++ {
		pad[i] = pad[i-1] + 1
	}
}

// reverse generates the reverse permutation pad from the given pad
// This allows for efficient decryption by reversing the permutation process
func reverse(pad []byte, rpad []byte) {
	for i := range pad {
		rpad[pad[i]] = byte(i)
	}
}

// seedToChunks converts the seed into 32-byte chunks based on the number of qubits
// This ensures that the seed is sufficiently long and has the required entropy
func seedToChunks(seed []byte, qubits uint8) [][]byte {
	// Ensure the seed length is at least 32 bytes
	if len(seed) < 32 {
		seed = pbkdf2.Key(seed, []byte(CHUNK_DERIVE_SALT), PBKDF2_LOOPS, 32, sha1.New)
	}

	// Calculate the required byte length for full permutation space
	byteLength := QPPMinimumSeedLength(qubits)
	chunkCount := (byteLength + 31) / 32 // round up to avoid entropy shortfall
	if chunkCount == 0 {
		chunkCount = 1
	}
	chunks := make([][]byte, chunkCount)
	for i := range chunks {
		chunks[i] = make([]byte, 32)
	}

	// Split the seed into overlapping chunks
	seedIdx := 0
	for i := range chunks {
		for j := range 32 {
			chunks[i][j] = seed[seedIdx%len(seed)]
			seedIdx++
		}

		// Perform key expansion
		derived := pbkdf2.Key(chunks[i], []byte(CHUNK_DERIVE_SALT), CHUNK_DERIVE_LOOPS, len(chunks[i]), sha1.New)
		copy(chunks[i], derived)
	}

	return chunks
}

// shuffle shuffles the pad based on the seed and pad identifier to create a permutation matrix
// It uses HMAC and PBKDF2 to derive a unique shuffle pattern from the seed and pad ID
func shuffle(chunk []byte, pad []byte, padID uint16, blocks []cipher.Block) {
	// use selected chunk based on pad ID to hmac the PAD_IDENTIFIER
	message := fmt.Sprintf(PAD_IDENTIFIER, padID)
	mac := hmac.New(sha256.New, chunk)
	mac.Write([]byte(message))
	sum := mac.Sum(nil)

	for i := len(pad) - 1; i > 0; i-- {
		// use all the entropy from the seed to generate a random number
		for j := range blocks {
			block := blocks[j%len(blocks)]
			for off := 0; off < len(sum); off += aes.BlockSize {
				block.Encrypt(sum[off:off+aes.BlockSize], sum[off:off+aes.BlockSize])
			}
		}
		bigrand := new(big.Int).SetBytes(sum)

		j := bigrand.Mod(bigrand, big.NewInt(int64(i+1))).Uint64()
		pad[i], pad[j] = pad[j], pad[i]
	}
}
