package qpp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
)

const PAD_IDENTIFIER = "QPP_%b"
const PM_SELECTOR_IDENTIFIER = "PERMUTATION_MATRIX_SELECTOR"
const NATIVE_BYTE_LENGTH = 8 // bit

type QuantumPermutationPad struct {
	pads  [][]byte // encryption
	rpads [][]byte // decryption

	numPads uint16     // number of pads
	qubits  uint8      // number of quantum bits
	encRand *rand.Rand // random source for pattern selection
	decRand *rand.Rand // random source for pattern selection
}

func NewQPP(seed []byte, numPads uint16, qubits uint8) *QuantumPermutationPad {
	qpp := &QuantumPermutationPad{
		numPads: numPads,
		qubits:  qubits,
	}

	qpp.pads = make([][]byte, numPads)
	qpp.rpads = make([][]byte, numPads)

	for i := uint16(0); i < numPads; i++ {
		qpp.pads[i] = make([]byte, 1<<qubits)
		qpp.rpads[i] = make([]byte, 1<<qubits)
		fill(qpp.pads[i])
		shuffle(seed, qpp.pads[i], i)
		reverse(qpp.pads[i], qpp.rpads[i])
	}

	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte(PM_SELECTOR_IDENTIFIER))
	sum := mac.Sum(nil)

	encSource := rand.NewSource(int64(binary.LittleEndian.Uint64(sum)))
	qpp.encRand = rand.New(encSource)
	decSource := rand.NewSource(int64(binary.LittleEndian.Uint64(sum)))
	qpp.decRand = rand.New(decSource)

	return qpp
}

// Encrypt encrypts data using the Quantum Permutation Pad
func (qpp *QuantumPermutationPad) Encrypt(data []byte) {
	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			index := qpp.encRand.Uint32() % uint32(qpp.numPads)
			pad := qpp.pads[index]
			data[i] = pad[data[i]]
		}
	default:
	}
}

func (qpp *QuantumPermutationPad) Decrypt(data []byte) {
	switch qpp.qubits {
	case NATIVE_BYTE_LENGTH:
		for i := 0; i < len(data); i++ {
			index := qpp.decRand.Uint32() % uint32(qpp.numPads)
			rpad := qpp.rpads[index]
			data[i] = rpad[data[i]]
		}
	default:
	}
}

func fill(pad []byte) {
	for i := 0; i < 256; i++ {
		pad[i] = byte(i)
	}
}

func reverse(pad []byte, rpad []byte) {
	for i := 0; i < 256; i++ {
		rpad[pad[i]] = byte(i)
	}
}

func shuffle(seed []byte, pad []byte, padID uint16) {
	message := fmt.Sprintf(PAD_IDENTIFIER, padID)
	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte(message))
	sum := mac.Sum(nil)
	source := rand.NewSource(int64(binary.LittleEndian.Uint64(sum)))

	rand.New(source).Shuffle(len(pad), func(i, j int) {
		pad[i], pad[j] = pad[j], pad[i]
	})
}
