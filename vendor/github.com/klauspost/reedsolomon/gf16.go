package reedsolomon

// GF16Mul multiplies two GF(2^16) elements.
// Uses GF(2^16) with polynomial 0x1002D.
// Initializes lazily on first use.
func (l LowLevel) GF16Mul(a, b uint16) uint16 {
	initConstants()
	if a == 0 || b == 0 {
		return 0
	}
	logSum := addMod(logLUT[ffe(a)], logLUT[ffe(b)])
	if logSum >= modulus {
		logSum -= modulus
	}
	return uint16(expLUT[logSum])
}
