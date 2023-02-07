// Copyright (c) 2019. Temple3x (temple3x@gmail.com)
//
// Use of this source code is governed by the MIT License
// that can be found in the LICENSE file.
 
#define dst BX // parity's address
#define d2src SI // two-dimension src_slice's address
#define csrc CX // cnt of src
#define len DX // len of vect
#define pos R8 // job position in vect

#define csrc_tmp R9
#define d2src_off R10
#define src_tmp R11
#define not_aligned_len R12
#define src_val0 R13
#define src_val1 R14

// func encodeAVX512(dst []byte, src [][]byte)
TEXT ·encodeAVX512(SB), 4, $0
	MOVQ  d+0(FP), dst
	MOVQ  src+24(FP), d2src
	MOVQ  c+32(FP), csrc
	MOVQ  l+8(FP), len
	TESTQ $255, len
	JNZ   not_aligned

aligned:
	MOVQ $0, pos

loop256b:
	MOVQ     csrc, csrc_tmp                // store src_cnt -> csrc_tmp
	SUBQ     $2, csrc_tmp
	MOVQ     $0, d2src_off
	MOVQ     (d2src)(d2src_off*1), src_tmp // get first src_vect's addr -> src_tmp
	VMOVDQU8 (src_tmp)(pos*1), Z0
	VMOVDQU8 64(src_tmp)(pos*1), Z1
	VMOVDQU8 128(src_tmp)(pos*1), Z2
	VMOVDQU8 192(src_tmp)(pos*1), Z3

next_vect:
	ADDQ     $24, d2src_off                // len(slice) = 24
	MOVQ     (d2src)(d2src_off*1), src_tmp // next data_vect
	VMOVDQU8 (src_tmp)(pos*1), Z4
	VMOVDQU8 64(src_tmp)(pos*1), Z5
	VMOVDQU8 128(src_tmp)(pos*1), Z6
	VMOVDQU8 192(src_tmp)(pos*1), Z7
	VPXORQ   Z4, Z0, Z0
	VPXORQ   Z5, Z1, Z1
	VPXORQ   Z6, Z2, Z2
	VPXORQ   Z7, Z3, Z3
	SUBQ     $1, csrc_tmp
	JGE      next_vect

	VMOVDQU8 Z0, (dst)(pos*1)
	VMOVDQU8 Z1, 64(dst)(pos*1)
	VMOVDQU8 Z2, 128(dst)(pos*1)
	VMOVDQU8 Z3, 192(dst)(pos*1)

	ADDQ $256, pos
	CMPQ len, pos
	JNE  loop256b
	VZEROUPPER
	RET

loop_1b:
	MOVQ csrc, csrc_tmp
	MOVQ $0, d2src_off
	MOVQ (d2src)(d2src_off*1), src_tmp
	SUBQ $2, csrc_tmp
	MOVB -1(src_tmp)(len*1), src_val0  // encode from the end of src

next_vect_1b:
	ADDQ $24, d2src_off
	MOVQ (d2src)(d2src_off*1), src_tmp
	MOVB -1(src_tmp)(len*1), src_val1
	XORB src_val1, src_val0
	SUBQ $1, csrc_tmp
	JGE  next_vect_1b

	MOVB  src_val0, -1(dst)(len*1)
	SUBQ  $1, len
	TESTQ $7, len
	JNZ   loop_1b

	CMPQ  len, $0
	JE    ret
	TESTQ $255, len
	JZ    aligned

not_aligned:
	TESTQ $7, len
	JNE   loop_1b
	MOVQ  len, not_aligned_len
	ANDQ  $255, not_aligned_len

loop_8b:
	MOVQ csrc, csrc_tmp
	MOVQ $0, d2src_off
	MOVQ (d2src)(d2src_off*1), src_tmp
	SUBQ $2, csrc_tmp
	MOVQ -8(src_tmp)(len*1), src_val0

next_vect_8b:
	ADDQ $24, d2src_off
	MOVQ (d2src)(d2src_off*1), src_tmp
	MOVQ -8(src_tmp)(len*1), src_val1
	XORQ src_val1, src_val0
	SUBQ $1, csrc_tmp
	JGE  next_vect_8b

	MOVQ src_val0, -8(dst)(len*1)
	SUBQ $8, len
	SUBQ $8, not_aligned_len
	JG   loop_8b

	CMPQ len, $256
	JGE  aligned
	RET

ret:
	RET
