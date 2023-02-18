//+build !noasm
//+build !appengine
//+build !gccgo

// Copyright 2015, Klaus Post, see LICENSE for details.
// Copyright 2019, Minio, Inc.

#define LOAD(OFFSET) \
	MOVQ      OFFSET(SI), BX  \
	VMOVDQU64 (BX)(R11*1), Z0 \
	VPSRLQ    $4, Z0, Z1      \ // high input
	VPANDQ    Z2, Z0, Z0      \ // low input
	VPANDQ    Z2, Z1, Z1      // high input

#define GALOIS_MUL(MUL_LO, MUL_HI, LO, HI, OUT) \
	VPSHUFB    Z0, MUL_LO, LO     \ // mul low part
	VPSHUFB    Z1, MUL_HI, HI     \ // mul high part
	VPTERNLOGD $0x96, LO, HI, OUT

#define GALOIS(C1, C2, IN, LO, HI, OUT) \
	VSHUFI64X2 $C1, IN, IN, LO      \
	VSHUFI64X2 $C2, IN, IN, HI      \
	GALOIS_MUL(LO, HI, LO, HI, OUT)

//
// Process single output row from a total of 8 input rows
//
// func _galMulAVX512Parallel81(in, out [][]byte, matrix *[matrixSize81]byte, addTo bool)
TEXT ·_galMulAVX512Parallel81(SB), 7, $0
	MOVQ  in+0(FP), SI
	MOVQ  8(SI), R9              // R9: len(in)
	SHRQ  $6, R9                 // len(in) / 64
	TESTQ R9, R9
	JZ    done_avx512_parallel81

	MOVQ      matrix+48(FP), SI
	VMOVDQU64 0x000(SI), Z16
	VMOVDQU64 0x040(SI), Z17
	VMOVDQU64 0x080(SI), Z18
	VMOVDQU64 0x0c0(SI), Z19

	// Initialize multiplication constants
	VSHUFI64X2 $0x55, Z16, Z16, Z20
	VSHUFI64X2 $0xaa, Z16, Z16, Z24
	VSHUFI64X2 $0xff, Z16, Z16, Z28
	VSHUFI64X2 $0x00, Z16, Z16, Z16

	VSHUFI64X2 $0x55, Z17, Z17, Z21
	VSHUFI64X2 $0xaa, Z17, Z17, Z25
	VSHUFI64X2 $0xff, Z17, Z17, Z29
	VSHUFI64X2 $0x00, Z17, Z17, Z17

	VSHUFI64X2 $0x55, Z18, Z18, Z22
	VSHUFI64X2 $0xaa, Z18, Z18, Z26
	VSHUFI64X2 $0xff, Z18, Z18, Z30
	VSHUFI64X2 $0x00, Z18, Z18, Z18

	VSHUFI64X2 $0x55, Z19, Z19, Z23
	VSHUFI64X2 $0xaa, Z19, Z19, Z27
	VSHUFI64X2 $0xff, Z19, Z19, Z31
	VSHUFI64X2 $0x00, Z19, Z19, Z19

	MOVQ         $15, BX
	VPBROADCASTB BX, Z2

	MOVB  addTo+56(FP), AX
	IMULQ $-0x1, AX
	KMOVQ AX, K1
	MOVQ  in+0(FP), SI     // SI: &in
	MOVQ  in_len+8(FP), AX // number of inputs
	XORQ  R11, R11
	MOVQ  out+24(FP), DX
	MOVQ  (DX), DX         // DX: &out[0][0]

loopback_avx512_parallel81:
	VMOVDQU64.Z (DX), K1, Z4

	LOAD(0x00)                         // &in[0][0]
	GALOIS_MUL(Z16, Z20, Z14, Z15, Z4)

	CMPQ AX, $1
	JE   skip_avx512_parallel81

	LOAD(0x18)                         // &in[1][0]
	GALOIS_MUL(Z24, Z28, Z14, Z15, Z4)

	CMPQ AX, $2
	JE   skip_avx512_parallel81

	LOAD(0x30)                         // &in[2][0]
	GALOIS_MUL(Z17, Z21, Z14, Z15, Z4)

	CMPQ AX, $3
	JE   skip_avx512_parallel81

	LOAD(0x48)                         // &in[3][0]
	GALOIS_MUL(Z25, Z29, Z14, Z15, Z4)

	CMPQ AX, $4
	JE   skip_avx512_parallel81

	LOAD(0x60)                         // &in[4][0]
	GALOIS_MUL(Z18, Z22, Z14, Z15, Z4)

	CMPQ AX, $5
	JE   skip_avx512_parallel81

	LOAD(0x78)                         // &in[5][0]
	GALOIS_MUL(Z26, Z30, Z14, Z15, Z4)

	CMPQ AX, $6
	JE   skip_avx512_parallel81

	LOAD(0x90)                         // &in[6][0]
	GALOIS_MUL(Z19, Z23, Z14, Z15, Z4)

	CMPQ AX, $7
	JE   skip_avx512_parallel81

	LOAD(0xa8)                         // &in[7][0]
	GALOIS_MUL(Z27, Z31, Z14, Z15, Z4)

skip_avx512_parallel81:
	VMOVDQU64 Z4, (DX)

	ADDQ $64, R11 // in4+=64

	ADDQ $64, DX // out+=64

	SUBQ $1, R9
	JNZ  loopback_avx512_parallel81

done_avx512_parallel81:
	VZEROUPPER
	RET

//
// Process 2 output rows in parallel from a total of 8 input rows
//
// func _galMulAVX512Parallel82(in, out [][]byte, matrix *[matrixSize82]byte, addTo bool)
TEXT ·_galMulAVX512Parallel82(SB), 7, $0
	MOVQ  in+0(FP), SI
	MOVQ  8(SI), R9              // R9: len(in)
	SHRQ  $6, R9                 // len(in) / 64
	TESTQ R9, R9
	JZ    done_avx512_parallel82

	MOVQ      matrix+48(FP), SI
	VMOVDQU64 0x000(SI), Z16
	VMOVDQU64 0x040(SI), Z17
	VMOVDQU64 0x080(SI), Z18
	VMOVDQU64 0x0c0(SI), Z19
	VMOVDQU64 0x100(SI), Z20
	VMOVDQU64 0x140(SI), Z21
	VMOVDQU64 0x180(SI), Z22
	VMOVDQU64 0x1c0(SI), Z23

	// Initialize multiplication constants
	VSHUFI64X2 $0x55, Z16, Z16, Z24
	VSHUFI64X2 $0xaa, Z16, Z16, Z25
	VSHUFI64X2 $0xff, Z16, Z16, Z26
	VSHUFI64X2 $0x00, Z16, Z16, Z16

	VSHUFI64X2 $0x55, Z20, Z20, Z27
	VSHUFI64X2 $0xaa, Z20, Z20, Z28
	VSHUFI64X2 $0xff, Z20, Z20, Z29
	VSHUFI64X2 $0x00, Z20, Z20, Z20

	VSHUFI64X2 $0x55, Z17, Z17, Z30
	VSHUFI64X2 $0xaa, Z17, Z17, Z31
	VSHUFI64X2 $0xff, Z17, Z17, Z11
	VSHUFI64X2 $0x00, Z17, Z17, Z17

	VSHUFI64X2 $0x55, Z21, Z21, Z8
	VSHUFI64X2 $0xaa, Z21, Z21, Z9
	VSHUFI64X2 $0xff, Z21, Z21, Z10
	VSHUFI64X2 $0x00, Z21, Z21, Z21

	MOVQ         $15, BX
	VPBROADCASTB BX, Z2

	MOVB  addTo+56(FP), AX
	IMULQ $-0x1, AX
	KMOVQ AX, K1
	MOVQ  in+0(FP), SI     // SI: &in
	MOVQ  in_len+8(FP), AX // number of inputs
	XORQ  R11, R11
	MOVQ  out+24(FP), DX
	MOVQ  24(DX), CX       // CX: &out[1][0]
	MOVQ  (DX), DX         // DX: &out[0][0]

loopback_avx512_parallel82:
	VMOVDQU64.Z (DX), K1, Z4
	VMOVDQU64.Z (CX), K1, Z5

	LOAD(0x00)                         // &in[0][0]
	GALOIS_MUL(Z16, Z24, Z14, Z15, Z4)
	GALOIS_MUL(Z20, Z27, Z12, Z13, Z5)

	CMPQ AX, $1
	JE   skip_avx512_parallel82

	LOAD(0x18)                         // &in[1][0]
	GALOIS_MUL(Z25, Z26, Z14, Z15, Z4)
	GALOIS_MUL(Z28, Z29, Z12, Z13, Z5)

	CMPQ AX, $2
	JE   skip_avx512_parallel82

	LOAD(0x30)                         // &in[2][0]
	GALOIS_MUL(Z17, Z30, Z14, Z15, Z4)
	GALOIS_MUL(Z21, Z8, Z12, Z13, Z5)

	CMPQ AX, $3
	JE   skip_avx512_parallel82

	LOAD(0x48)                         // &in[3][0]
	GALOIS_MUL(Z31, Z11, Z14, Z15, Z4)
	GALOIS_MUL(Z9, Z10, Z12, Z13, Z5)

	CMPQ AX, $4
	JE   skip_avx512_parallel82

	LOAD(0x60)                            // &in[4][0]
	GALOIS(0x00, 0x55, Z18, Z14, Z15, Z4)
	GALOIS(0x00, 0x55, Z22, Z12, Z13, Z5)

	CMPQ AX, $5
	JE   skip_avx512_parallel82

	LOAD(0x78)                            // &in[5][0]
	GALOIS(0xaa, 0xff, Z18, Z14, Z15, Z4)
	GALOIS(0xaa, 0xff, Z22, Z12, Z13, Z5)

	CMPQ AX, $6
	JE   skip_avx512_parallel82

	LOAD(0x90)                            // &in[6][0]
	GALOIS(0x00, 0x55, Z19, Z14, Z15, Z4)
	GALOIS(0x00, 0x55, Z23, Z12, Z13, Z5)

	CMPQ AX, $7
	JE   skip_avx512_parallel82

	LOAD(0xa8)                            // &in[7][0]
	GALOIS(0xaa, 0xff, Z19, Z14, Z15, Z4)
	GALOIS(0xaa, 0xff, Z23, Z12, Z13, Z5)

skip_avx512_parallel82:
	VMOVDQU64 Z4, (DX)
	VMOVDQU64 Z5, (CX)

	ADDQ $64, R11 // in4+=64

	ADDQ $64, DX // out+=64
	ADDQ $64, CX // out2+=64

	SUBQ $1, R9
	JNZ  loopback_avx512_parallel82

done_avx512_parallel82:
	VZEROUPPER
	RET

//
// Process 4 output rows in parallel from a total of 8 input rows
//
// func _galMulAVX512Parallel84(in, out [][]byte, matrix *[matrixSize84]byte, addTo bool)
TEXT ·_galMulAVX512Parallel84(SB), 7, $0
	MOVQ  in+0(FP), SI
	MOVQ  8(SI), R9              // R9: len(in)
	SHRQ  $6, R9                 // len(in) / 64
	TESTQ R9, R9
	JZ    done_avx512_parallel84

	MOVQ      matrix+48(FP), SI
	VMOVDQU64 0x000(SI), Z16
	VMOVDQU64 0x040(SI), Z17
	VMOVDQU64 0x080(SI), Z18
	VMOVDQU64 0x0c0(SI), Z19
	VMOVDQU64 0x100(SI), Z20
	VMOVDQU64 0x140(SI), Z21
	VMOVDQU64 0x180(SI), Z22
	VMOVDQU64 0x1c0(SI), Z23
	VMOVDQU64 0x200(SI), Z24
	VMOVDQU64 0x240(SI), Z25
	VMOVDQU64 0x280(SI), Z26
	VMOVDQU64 0x2c0(SI), Z27
	VMOVDQU64 0x300(SI), Z28
	VMOVDQU64 0x340(SI), Z29
	VMOVDQU64 0x380(SI), Z30
	VMOVDQU64 0x3c0(SI), Z31

	MOVQ         $15, BX
	VPBROADCASTB BX, Z2

	MOVB  addTo+56(FP), AX
	IMULQ $-0x1, AX
	KMOVQ AX, K1
	MOVQ  in+0(FP), SI     // SI: &in
	MOVQ  in_len+8(FP), AX // number of inputs
	XORQ  R11, R11
	MOVQ  out+24(FP), DX
	MOVQ  24(DX), CX       // CX: &out[1][0]
	MOVQ  48(DX), R10      // R10: &out[2][0]
	MOVQ  72(DX), R12      // R12: &out[3][0]
	MOVQ  (DX), DX         // DX: &out[0][0]

loopback_avx512_parallel84:
	VMOVDQU64.Z (DX), K1, Z4
	VMOVDQU64.Z (CX), K1, Z5
	VMOVDQU64.Z (R10), K1, Z6
	VMOVDQU64.Z (R12), K1, Z7

	LOAD(0x00)                            // &in[0][0]
	GALOIS(0x00, 0x55, Z16, Z14, Z15, Z4)
	GALOIS(0x00, 0x55, Z20, Z12, Z13, Z5)
	GALOIS(0x00, 0x55, Z24, Z10, Z11, Z6)
	GALOIS(0x00, 0x55, Z28,  Z8,  Z9, Z7)

	CMPQ AX, $1
	JE   skip_avx512_parallel84

	LOAD(0x18)                            // &in[1][0]
	GALOIS(0xaa, 0xff, Z16, Z14, Z15, Z4)
	GALOIS(0xaa, 0xff, Z20, Z12, Z13, Z5)
	GALOIS(0xaa, 0xff, Z24, Z10, Z11, Z6)
	GALOIS(0xaa, 0xff, Z28,  Z8,  Z9, Z7)

	CMPQ AX, $2
	JE   skip_avx512_parallel84

	LOAD(0x30)                            // &in[2][0]
	GALOIS(0x00, 0x55, Z17, Z14, Z15, Z4)
	GALOIS(0x00, 0x55, Z21, Z12, Z13, Z5)
	GALOIS(0x00, 0x55, Z25, Z10, Z11, Z6)
	GALOIS(0x00, 0x55, Z29,  Z8,  Z9, Z7)

	CMPQ AX, $3
	JE   skip_avx512_parallel84

	LOAD(0x48)                            // &in[3][0]
	GALOIS(0xaa, 0xff, Z17, Z14, Z15, Z4)
	GALOIS(0xaa, 0xff, Z21, Z12, Z13, Z5)
	GALOIS(0xaa, 0xff, Z25, Z10, Z11, Z6)
	GALOIS(0xaa, 0xff, Z29,  Z8,  Z9, Z7)

	CMPQ AX, $4
	JE   skip_avx512_parallel84

	LOAD(0x60)                            // &in[4][0]
	GALOIS(0x00, 0x55, Z18, Z14, Z15, Z4)
	GALOIS(0x00, 0x55, Z22, Z12, Z13, Z5)
	GALOIS(0x00, 0x55, Z26, Z10, Z11, Z6)
	GALOIS(0x00, 0x55, Z30,  Z8,  Z9, Z7)

	CMPQ AX, $5
	JE   skip_avx512_parallel84

	LOAD(0x78)                            // &in[5][0]
	GALOIS(0xaa, 0xff, Z18, Z14, Z15, Z4)
	GALOIS(0xaa, 0xff, Z22, Z12, Z13, Z5)
	GALOIS(0xaa, 0xff, Z26, Z10, Z11, Z6)
	GALOIS(0xaa, 0xff, Z30,  Z8,  Z9, Z7)

	CMPQ AX, $6
	JE   skip_avx512_parallel84

	LOAD(0x90)                            // &in[6][0]
	GALOIS(0x00, 0x55, Z19, Z14, Z15, Z4)
	GALOIS(0x00, 0x55, Z23, Z12, Z13, Z5)
	GALOIS(0x00, 0x55, Z27, Z10, Z11, Z6)
	GALOIS(0x00, 0x55, Z31,  Z8,  Z9, Z7)

	CMPQ AX, $7
	JE   skip_avx512_parallel84

	LOAD(0xa8)                            // &in[7][0]
	GALOIS(0xaa, 0xff, Z19, Z14, Z15, Z4)
	GALOIS(0xaa, 0xff, Z23, Z12, Z13, Z5)
	GALOIS(0xaa, 0xff, Z27, Z10, Z11, Z6)
	GALOIS(0xaa, 0xff, Z31,  Z8,  Z9, Z7)

skip_avx512_parallel84:
	VMOVDQU64 Z4, (DX)
	VMOVDQU64 Z5, (CX)
	VMOVDQU64 Z6, (R10)
	VMOVDQU64 Z7, (R12)

	ADDQ $64, R11 // in4+=64

	ADDQ $64, DX  // out+=64
	ADDQ $64, CX  // out2+=64
	ADDQ $64, R10 // out3+=64
	ADDQ $64, R12 // out4+=64

	SUBQ $1, R9
	JNZ  loopback_avx512_parallel84

done_avx512_parallel84:
	VZEROUPPER
	RET
