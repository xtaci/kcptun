package qpp

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"

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
)

// QuantumPermutationPad represents the encryption/decryption structure using quantum permutation pads
// QPP is a cryptographic technique that leverages quantum-inspired permutation matrices to provide secure encryption.
type QuantumPermutationPad struct {
	pads    [][]byte   // Encryption pads, each pad is a permutation matrix for encryption
	rpads   [][]byte   // Decryption pads, each pad is a reverse permutation matrix for decryption
	numPads uint16     // Number of pads (permutation matrices)
	qubits  uint8      // Number of quantum bits, determines the size of each pad
	encRand *rand.Rand // Default random source for encryption pad selection
	decRand *rand.Rand // Default random source for decryption pad selection
}

// NewQPP creates a new Quantum Permutation Pad instance with the provided seed, number of pads, and qubits
// The seed is used to generate deterministic pseudo-random number generators (PRNGs) for both encryption and decryption
func NewQPP(seed []byte, numPads uint16, qubits uint8) *QuantumPermutationPad {
	qpp := &QuantumPermutationPad{
		numPads: numPads,
		qubits:  qubits,
	}

	qpp.pads = make([][]byte, numPads)
	qpp.rpads = make([][]byte, numPads)

	// Initialize and shuffle pads to create permutation matrices
	for i := uint16(0); i < numPads; i++ {
		qpp.pads[i] = make([]byte, 1<<qubits)
		qpp.rpads[i] = make([]byte, 1<<qubits)
		fill(qpp.pads[i])                  // Fill pad with sequential byte values
		shuffle(seed, qpp.pads[i], i)      // Shuffle pad to create a unique permutation matrix
		reverse(qpp.pads[i], qpp.rpads[i]) // Create the reverse permutation matrix for decryption
	}

	qpp.encRand = qpp.CreatePRNG(seed) // Create default PRNG for encryption
	qpp.decRand = qpp.CreatePRNG(seed)

	return qpp
}

// Encrypt encrypts the given data using the Quantum Permutation Pad with the default PRNG
// It selects a permutation matrix based on a random index and uses it to permute each byte of the data
func (qpp *QuantumPermutationPad) Encrypt(data []byte) {
	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			rand := qpp.encRand.Uint32()           // Generate a pseudo-random number
			index := rand % uint32(qpp.numPads)    // Select a permutation matrix index
			pad := qpp.pads[index]                 // Retrieve the permutation matrix
			data[i] = pad[data[i]^byte(rand&0xFF)] // Apply the permutation to the data byte
		}
	default:
	}
}

// Decrypt decrypts the given data using the Quantum Permutation Pad with the default PRNG
// It selects a reverse permutation matrix based on a random index and uses it to restore each byte of the data
func (qpp *QuantumPermutationPad) Decrypt(data []byte) {
	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			rand := qpp.decRand.Uint32()
			index := rand % uint32(qpp.numPads)
			rpad := qpp.rpads[index]
			data[i] = rpad[data[i]] ^ byte(rand&0xFF)
		}
	default:
	}
}

// CreatePRNG creates a deterministic pseudo-random number generator based on the provided seed
// It uses HMAC and PBKDF2 to derive a random seed for the PRNG
func (qpp *QuantumPermutationPad) CreatePRNG(seed []byte) *rand.Rand {
	// condense entropy from seed to 8 bytes
	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte(PM_SELECTOR_IDENTIFIER))
	sum := mac.Sum(nil)
	dk := pbkdf2.Key(sum, []byte(PRNG_SALT), PBKDF2_LOOPS, 8, sha1.New)
	source := rand.NewSource(int64(binary.LittleEndian.Uint64(dk)))
	return rand.New(source)
}

// EncryptWithPRNG encrypts the data using the Quantum Permutation Pad with a custom PRNG
// This function shares the same permutation matrices
func (qpp *QuantumPermutationPad) EncryptWithPRNG(data []byte, rand *rand.Rand) {
	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			rand := rand.Uint32()
			index := rand % uint32(qpp.numPads)
			pad := qpp.pads[index]
			data[i] = pad[data[i]^byte(rand&0xFF)]
		}
	default:
	}
}

// DecryptWithPRNG decrypts the data using the Quantum Permutation Pad with a custom PRNG
// This function shares the same permutation matrices
func (qpp *QuantumPermutationPad) DecryptWithPRNG(data []byte, rand *rand.Rand) {
	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			rand := rand.Uint32()
			index := rand % uint32(qpp.numPads)
			rpad := qpp.rpads[index]
			data[i] = rpad[data[i]] ^ byte(rand&0xFF)
		}
	default:
	}
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

// shuffle shuffles the pad based on the seed and pad identifier to create a permutation matrix
// It uses HMAC and PBKDF2 to derive a unique shuffle pattern from the seed and pad ID
func shuffle(seed []byte, pad []byte, padID uint16) {
	message := fmt.Sprintf(PAD_IDENTIFIER, padID)
	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte(message))
	sum := mac.Sum(nil)

	// expand seed to 32-bytes for AES-based PRNG
	aeskey := pbkdf2.Key(seed, []byte(SHUFFLE_SALT), PBKDF2_LOOPS, 32, sha1.New)
	block, _ := aes.NewCipher(aeskey)
	for i := len(pad) - 1; i > 0; i-- {
		block.Encrypt(sum, sum)
		j := binary.LittleEndian.Uint64(sum) % uint64(i+1)
		pad[i], pad[j] = pad[j], pad[i]
	}
}
