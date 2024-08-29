// Package qpp implements Quantum permutation pad
//
// Quantum permutation pad or QPP is a quantum-safe symmetric cryptographic
// algorithm proposed by Kuang and Bettenburg in 2020. The theoretical
// foundation of QPP leverages the linear algebraic representations of
// quantum gates which makes QPP realizable in both, quantum and classical
// systems. By applying the QPP with 64 of 8-bit permutation gates, holding
// respective entropy of over 100,000 bits, they accomplished quantum random
// number distributions digitally over today’s classical internet. The QPP has
// also been used to create pseudo quantum random numbers and served as a
// foundation for quantum-safe lightweight block and streaming ciphers.
//
// This file implements QPP in 8-qubits, which is compatible with the classical
// architecture. In 8-qubits, the overall permutation matrix reaches 256!.
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
	MAGIC                  = 0x1A2B3C4D5E6F7890
	PAD_SWITCH             = 8 // switch pad for every PAD_SWITCH bytes
	QUBITS                 = 8 // number of quantum bits of this implementation
)

// Rand is a stateful random number generator
type Rand struct {
	xoshiro [4]uint64
	seed64  uint64
	count   uint8
}

// QuantumPermutationPad represents the encryption/decryption structure using quantum permutation pads
// QPP is a cryptographic technique that leverages quantum-inspired permutation matrices to provide secure encryption.
type QuantumPermutationPad struct {
	pads     []byte  // Encryption pads, each pad is a permutation matrix for encryption
	rpads    []byte  // Decryption pads, each pad is a reverse permutation matrix for decryption
	padsPtr  uintptr // raw pointer to encryption pads
	rpadsPtr uintptr // raw pointer to decryption pads

	numPads uint16 // Number of pads (permutation matrices)
	encRand *Rand  // Default random source for encryption pad selection
	decRand *Rand  // Default random source for decryption pad selection
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
	qpp.padsPtr = uintptr(unsafe.Pointer(unsafe.SliceData(qpp.pads)))
	qpp.rpadsPtr = uintptr(unsafe.Pointer(unsafe.SliceData(qpp.rpads)))

	chunks := seedToChunks(seed, QUBITS)
	// creat AES-256 blocks to generate random number for shuffling
	var blocks []cipher.Block
	for i := range chunks {
		aeskey := pbkdf2.Key(chunks[i], []byte(SHUFFLE_SALT), PBKDF2_LOOPS, 32, sha1.New)
		block, _ := aes.NewCipher(aeskey)
		blocks = append(blocks, block)
	}

	// Initialize and shuffle pads to create permutation matrices
	for i := 0; i < int(numPads); i++ {
		pad := qpp.pads[i*matrixBytes : (i+1)*matrixBytes]
		rpad := qpp.rpads[i*matrixBytes : (i+1)*matrixBytes]

		// Fill pad with sequential byte values
		fill(pad)
		// Shuffle pad to create a unique permutation matrix
		shuffle(chunks[i%len(chunks)], QUBITS, pad, uint16(i), blocks)
		// Create the reverse permutation matrix for decryption
		reverse(pad, rpad)
	}

	qpp.encRand = qpp.CreatePRNG(seed) // Create default PRNG for encryption
	qpp.decRand = qpp.CreatePRNG(seed) // Create default PRNG for decryption

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
func (qpp *QuantumPermutationPad) CreatePRNG(seed []byte) *Rand {
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

// EncryptWithPRNG encrypts the data using the Quantum Permutation Pad with a custom PRNG
// This function shares the same permutation matrices
func (qpp *QuantumPermutationPad) EncryptWithPRNG(data []byte, rand *Rand) {
	// initial r, index, count
	size := len(data)
	r := rand.seed64
	base := qpp.padsPtr + uintptr(uint16(r)%qpp.numPads)<<8
	count := rand.count
	var rr byte

	// handle unaligned 8bytes
	if count != 0 {
		offset := 0
		for ; offset < len(data); offset++ {
			// using r as the base random number
			rr = byte(r >> (count * 8))
			data[offset] = *(*byte)(unsafe.Pointer(base + uintptr(data[offset]^rr)))
			count++

			// switch to another pad when count reaches PAD_SWITCH
			if count == PAD_SWITCH {
				// switch to another pad
				r = xoshiro256ss(&rand.xoshiro)
				base = qpp.padsPtr + uintptr(uint16(r)%qpp.numPads)<<8
				offset = offset + 1
				count = 0
				break
			}
		}
		data = data[offset:] // aligned bytes start from here
	}

	// handle 8-bytes aligned
	repeat := len(data) / 8
	for i := 0; i < repeat; i++ {
		d := data[i*8 : i*8+8]
		rr0 := byte(r >> 0)
		rr1 := byte(r >> 8)
		rr2 := byte(r >> 16)
		rr3 := byte(r >> 24)
		rr4 := byte(r >> 32)
		rr5 := byte(r >> 40)
		rr6 := byte(r >> 48)
		rr7 := byte(r >> 56)

		d[0] = *(*byte)(unsafe.Pointer(base + uintptr(d[0]^rr0)))
		d[1] = *(*byte)(unsafe.Pointer(base + uintptr(d[1]^rr1)))
		d[2] = *(*byte)(unsafe.Pointer(base + uintptr(d[2]^rr2)))
		d[3] = *(*byte)(unsafe.Pointer(base + uintptr(d[3]^rr3)))
		d[4] = *(*byte)(unsafe.Pointer(base + uintptr(d[4]^rr4)))
		d[5] = *(*byte)(unsafe.Pointer(base + uintptr(d[5]^rr5)))
		d[6] = *(*byte)(unsafe.Pointer(base + uintptr(d[6]^rr6)))
		d[7] = *(*byte)(unsafe.Pointer(base + uintptr(d[7]^rr7)))

		r = xoshiro256ss(&rand.xoshiro)
		base = qpp.padsPtr + uintptr(uint16(r)%qpp.numPads)<<8
	}
	data = data[repeat*8:]

	// handle remaining unaligned bytes
	for i := 0; i < len(data); i++ {
		rr = byte(r >> (count * 8))
		data[i] = *(*byte)(unsafe.Pointer(base + uintptr(data[i]^byte(rr))))
		count++
	}

	// set back r & count
	rand.seed64 = uint64(r)
	rand.count = uint8((int(rand.count) + size) % PAD_SWITCH)
}

// DecryptWithPRNG decrypts the data using the Quantum Permutation Pad with a custom PRNG
// This function shares the same permutation matrices
func (qpp *QuantumPermutationPad) DecryptWithPRNG(data []byte, rand *Rand) {
	size := len(data)
	r := rand.seed64
	base := qpp.rpadsPtr + uintptr(uint16(r)%qpp.numPads)<<8
	count := rand.count
	var rr byte

	// handle unaligned 8bytes
	if count != 0 {
		offset := 0
		for ; offset < len(data); offset++ {
			rr = byte(r >> (count * 8))
			data[offset] = *(*byte)(unsafe.Pointer(base + uintptr(data[offset]))) ^ rr
			count++

			if count == PAD_SWITCH {
				r = xoshiro256ss(&rand.xoshiro)
				base = qpp.rpadsPtr + uintptr(uint16(r)%qpp.numPads)<<8
				offset = offset + 1
				count = 0
				break
			}
		}
		data = data[offset:]
	}

	// handle 8-bytes aligned
	repeat := len(data) / 8
	for i := 0; i < repeat; i++ {
		d := data[i*8 : i*8+8]
		rr0 := byte(r >> 0)
		rr1 := byte(r >> 8)
		rr2 := byte(r >> 16)
		rr3 := byte(r >> 24)
		rr4 := byte(r >> 32)
		rr5 := byte(r >> 40)
		rr6 := byte(r >> 48)
		rr7 := byte(r >> 56)

		d[0] = *(*byte)(unsafe.Pointer(base + uintptr(d[0]))) ^ rr0
		d[1] = *(*byte)(unsafe.Pointer(base + uintptr(d[1]))) ^ rr1
		d[2] = *(*byte)(unsafe.Pointer(base + uintptr(d[2]))) ^ rr2
		d[3] = *(*byte)(unsafe.Pointer(base + uintptr(d[3]))) ^ rr3
		d[4] = *(*byte)(unsafe.Pointer(base + uintptr(d[4]))) ^ rr4
		d[5] = *(*byte)(unsafe.Pointer(base + uintptr(d[5]))) ^ rr5
		d[6] = *(*byte)(unsafe.Pointer(base + uintptr(d[6]))) ^ rr6
		d[7] = *(*byte)(unsafe.Pointer(base + uintptr(d[7]))) ^ rr7

		r = xoshiro256ss(&rand.xoshiro)
		base = qpp.rpadsPtr + uintptr(uint16(r)%qpp.numPads)<<8
	}
	data = data[repeat*8:]

	// handle remaining unaligned bytes
	for i := 0; i < len(data); i++ {
		rr = byte(r >> (count * 8))
		data[i] = *(*byte)(unsafe.Pointer(base + uintptr(data[i]))) ^ rr
		count++
	}

	// set back r & count
	rand.seed64 = r
	rand.count = uint8((int(rand.count) + size) % PAD_SWITCH)
}

// QPPMinimumSeedLength calculates the length required for the seed based on the number of qubits
// This ensures that the seed has sufficient entropy for the required permutations
func QPPMinimumSeedLength(qubits uint8) int {
	perms := big.NewInt(1 << qubits)
	for i := 1<<qubits - 1; i > 0; i-- {
		perms.Mul(perms, big.NewInt(int64(i)))
	}
	byteLen := perms.BitLen() / 8
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
	for i := 0; i < len(pad); i++ {
		pad[i] = byte(i)
	}
}

// reverse generates the reverse permutation pad from the given pad
// This allows for efficient decryption by reversing the permutation process
func reverse(pad []byte, rpad []byte) {
	for i := 0; i < len(pad); i++ {
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
	chunks := make([][]byte, byteLength/32)
	for i := 0; i < len(chunks); i++ {
		chunks[i] = make([]byte, 32)
	}

	// Split the seed into overlapping chunks
	seedIdx := 0
	for i := 0; i < len(chunks); i++ {
		for j := 0; j < 32; j++ {
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
func shuffle(chunk []byte, qubits uint8, pad []byte, padID uint16, blocks []cipher.Block) {
	// use selected chunk based on pad ID to hmac the PAD_IDENTIFIER
	message := fmt.Sprintf(PAD_IDENTIFIER, padID)
	mac := hmac.New(sha256.New, chunk)
	mac.Write([]byte(message))
	sum := mac.Sum(nil)

	for i := len(pad) - 1; i > 0; i-- {
		// use all the entropy from the seed to generate a random number
		for j := 0; j < len(blocks); j++ {
			block := blocks[j%len(blocks)]
			block.Encrypt(sum, sum)
		}
		bigrand := new(big.Int).SetBytes(sum)

		j := bigrand.Mod(bigrand, big.NewInt(int64(i+1))).Uint64()
		pad[i], pad[j] = pad[j], pad[i]
	}
}
