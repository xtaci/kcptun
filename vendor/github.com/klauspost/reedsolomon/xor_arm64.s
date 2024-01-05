//+build !noasm
//+build !appengine
//+build !gccgo

// func xorSliceNEON(in, out []byte)
TEXT ·xorSliceNEON(SB), 7, $0
	MOVD in_base+0(FP), R1
	MOVD in_len+8(FP), R2    // length of message
	MOVD out_base+24(FP), R5
	SUBS $32, R2
	BMI  completeXor

loopXor:
	// Main loop
	VLD1.P 32(R1), [V0.B16, V1.B16]
	VLD1   (R5), [V20.B16, V21.B16]

	VEOR V20.B16, V0.B16, V4.B16
	VEOR V21.B16, V1.B16, V5.B16

	// Store result
	VST1.P [V4.D2, V5.D2], 32(R5)

	SUBS $32, R2
	BPL  loopXor

completeXor:
	RET

