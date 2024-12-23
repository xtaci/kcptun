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
