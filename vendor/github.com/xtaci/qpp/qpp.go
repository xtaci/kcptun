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
)

// Rand is a stateful random number generator
type Rand struct {
	seed64 uint64
	count  uint8
}

// QuantumPermutationPad represents the encryption/decryption structure using quantum permutation pads
// QPP is a cryptographic technique that leverages quantum-inspired permutation matrices to provide secure encryption.
type QuantumPermutationPad struct {
	pads     []byte  // Encryption pads, each pad is a permutation matrix for encryption
	rpads    []byte  // Decryption pads, each pad is a reverse permutation matrix for decryption
	padsPtr  uintptr // raw pointer to encryption pads
	rpadsPtr uintptr // raw pointer to encryption pads

	numPads uint16 // Number of pads (permutation matrices)
	qubits  uint8  // Number of quantum bits, determines the size of each pad
	encRand *Rand  // Default random source for encryption pad selection
	decRand *Rand  // Default random source for decryption pad selection
}

// NewQPP creates a new Quantum Permutation Pad instance with the provided seed, number of pads, and qubits
// The seed is used to generate deterministic pseudo-random number generators (PRNGs) for both encryption and decryption
func NewQPP(seed []byte, numPads uint16, qubits uint8) *QuantumPermutationPad {
	qpp := &QuantumPermutationPad{
		numPads: numPads,
		qubits:  qubits,
	}

	matrixBytes := 1 << qubits
	qpp.pads = make([]byte, int(numPads)*matrixBytes)
	qpp.rpads = make([]byte, int(numPads)*matrixBytes)
	qpp.padsPtr = uintptr(unsafe.Pointer(unsafe.SliceData(qpp.pads)))
	qpp.rpadsPtr = uintptr(unsafe.Pointer(unsafe.SliceData(qpp.rpads)))

	chunks := seedToChunks(seed, qubits)
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
		shuffle(chunks[i%len(chunks)], qubits, pad, uint16(i), blocks)
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
	dk := pbkdf2.Key(sum, []byte(PRNG_SALT), PBKDF2_LOOPS, 8, sha1.New) // Derive a key for PRNG
	seed64 := binary.LittleEndian.Uint64(dk)
	if seed64 == 0 {
		seed64 = MAGIC
	}
	return &Rand{seed64: seed64} // Create and return PRNG
}

// EncryptWithPRNG encrypts the data using the Quantum Permutation Pad with a custom PRNG
// This function shares the same permutation matrices
func (qpp *QuantumPermutationPad) EncryptWithPRNG(data []byte, rand *Rand) {
	// initial r, index, count
	r := uint32(rand.seed64)
	index := uint16(r) % qpp.numPads // Select a permutation matrix index
	count := rand.count

	// loop
	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			// switch to another permutation pad for every 256 bytes
			if count%PAD_SWITCH == 0 {
				index = uint16(r) % qpp.numPads
				count = 0
			}

			offset := qpp.padsPtr + uintptr(index)<<8 + uintptr(data[i]^byte(r)) // Calculate the offset
			data[i] = *(*byte)(unsafe.Pointer(offset))                           // Apply the permutation to the data byte

			count++
			r = xorshift32(r)
		}

		// set back r & count
		rand.seed64 = uint64(r)
		rand.count += uint8(len(data) % PAD_SWITCH)
	default:
		// Handle other cases if needed
	}

}

// DecryptWithPRNG decrypts the data using the Quantum Permutation Pad with a custom PRNG
// This function shares the same permutation matrices
func (qpp *QuantumPermutationPad) DecryptWithPRNG(data []byte, rand *Rand) {
	r := uint32(rand.seed64)
	index := uint16(r) % qpp.numPads // Select a permutation matrix index
	count := rand.count

	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			if count%PAD_SWITCH == 0 {
				index = uint16(r) % qpp.numPads
				count = 0
			}

			offset := qpp.rpadsPtr + uintptr(index)<<8 + uintptr(data[i]) // Calculate the offset
			data[i] = *(*byte)(unsafe.Pointer(offset)) ^ byte(r)          // Apply the permutation to the data byte
			count++
			r = xorshift32(r)
		}

		// set back r & count
		rand.seed64 = uint64(r)
		rand.count += uint8(len(data) % PAD_SWITCH)
	default:
		// Handle other cases if needed
	}
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
