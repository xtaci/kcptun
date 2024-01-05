//+build !noasm
//+build !appengine
//+build !gccgo
//+build !nopshufb

// Copyright 2015, Klaus Post, see LICENSE for details.
// Copyright 2017, Minio, Inc.

#define LOAD(LO1, LO2, HI1, HI2) \
	VLD1.P 32(R1), [LO1.B16, LO2.B16] \
	                                  \
	\ // Get low input and high input
	VUSHR  $4, LO1.B16, HI1.B16       \
	VUSHR  $4, LO2.B16, HI2.B16       \
	VAND   V8.B16, LO1.B16, LO1.B16   \
	VAND   V8.B16, LO2.B16, LO2.B16

#define GALOIS_MUL(MUL_LO, MUL_HI, OUT1, OUT2, TMP1, TMP2) \
	\ // Mul low part and mul high part
	VTBL V0.B16, [MUL_LO.B16], OUT1.B16  \
	VTBL V10.B16, [MUL_HI.B16], OUT2.B16 \
	VTBL V1.B16, [MUL_LO.B16], TMP1.B16  \
	VTBL V11.B16, [MUL_HI.B16], TMP2.B16 \
	                                     \
	\ // Combine results
	VEOR OUT2.B16, OUT1.B16, OUT1.B16    \
	VEOR TMP2.B16, TMP1.B16, OUT2.B16

// func galMulNEON(low, high, in, out []byte)
TEXT ·galMulNEON(SB), 7, $0
	MOVD in_base+48(FP), R1
	MOVD in_len+56(FP), R2   // length of message
	MOVD out_base+72(FP), R5
	SUBS $32, R2
	BMI  complete

	MOVD low+0(FP), R10   // R10: &low
	MOVD high+24(FP), R11 // R11: &high
	VLD1 (R10), [V6.B16]
	VLD1 (R11), [V7.B16]

	//
	// Use an extra instruction below since `VDUP R3, V8.B16` generates assembler error
	// WORD $0x4e010c68 // dup v8.16b, w3
	//
	MOVD $0x0f, R3
	VMOV R3, V8.B[0]
	VDUP V8.B[0], V8.B16

loop:
	// Main loop
	LOAD(V0, V1, V10, V11)
	GALOIS_MUL(V6, V7, V4, V5, V14, V15)

	// Store result
	VST1.P [V4.D2, V5.D2], 32(R5)

	SUBS $32, R2
	BPL  loop

complete:
	RET

// func galMulXorNEON(low, high, in, out []byte)
TEXT ·galMulXorNEON(SB), 7, $0
	MOVD in_base+48(FP), R1
	MOVD in_len+56(FP), R2   // length of message
	MOVD out_base+72(FP), R5
	SUBS $32, R2
	BMI  completeXor

	MOVD low+0(FP), R10   // R10: &low
	MOVD high+24(FP), R11 // R11: &high
	VLD1 (R10), [V6.B16]
	VLD1 (R11), [V7.B16]

	//
	// Use an extra instruction below since `VDUP R3, V8.B16` generates assembler error
	// WORD $0x4e010c68 // dup v8.16b, w3
	//
	MOVD $0x0f, R3
	VMOV R3, V8.B[0]
	VDUP V8.B[0], V8.B16

loopXor:
	// Main loop
	VLD1 (R5), [V20.B16, V21.B16]

	LOAD(V0, V1, V10, V11)
	GALOIS_MUL(V6, V7, V4, V5, V14, V15)

	VEOR V20.B16, V4.B16, V4.B16
	VEOR V21.B16, V5.B16, V5.B16

	// Store result
	VST1.P [V4.D2, V5.D2], 32(R5)

	SUBS $32, R2
	BPL  loopXor

completeXor:
	RET
