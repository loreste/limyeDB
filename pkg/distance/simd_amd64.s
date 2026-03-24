#include "textflag.h"

// func dotProductAVX(a, b []float32) float32
TEXT ·dotProductAVX(SB), NOSPLIT, $0-52
    MOVQ a_base+0(FP), R8
    MOVQ a_len+8(FP), R9
    MOVQ b_base+24(FP), R10

    VXORPS X0, X0, X0
    XORQ R11, R11
    MOVQ R9, R12
    ANDQ $~7, R12

    CMPQ R12, $0
    JE dot_scalar_loop

dot_avx_loop:
    CMPQ R11, R12
    JGE dot_scalar_loop
    VMOVUPS (R8)(R11*4), Y1
    VMOVUPS (R10)(R11*4), Y2
    VMULPS Y2, Y1, Y2
    VADDPS Y2, Y0, Y0
    ADDQ $8, R11
    JMP dot_avx_loop

dot_scalar_loop:
    CMPQ R11, R9
    JGE dot_done
    VMOVSS (R8)(R11*4), X1
    VMOVSS (R10)(R11*4), X2
    VMULSS X2, X1, X2
    VADDSS X2, X0, X0
    ADDQ $1, R11
    JMP dot_scalar_loop

dot_done:
    VEXTRACTF128 $1, Y0, X1
    VADDPS X1, X0, X0
    VSHUFPS $0x0E, X0, X0, X1
    VADDPS X1, X0, X0
    VSHUFPS $0x01, X0, X0, X1
    VADDSS X1, X0, X0
    VMOVSS X0, ret+48(FP)
    VZEROUPPER
    RET


// func euclideanDistanceAVX(a, b []float32) float32
TEXT ·euclideanDistanceAVX(SB), NOSPLIT, $0-52
    MOVQ a_base+0(FP), R8
    MOVQ a_len+8(FP), R9
    MOVQ b_base+24(FP), R10

    VXORPS X0, X0, X0
    XORQ R11, R11
    MOVQ R9, R12
    ANDQ $~7, R12

    CMPQ R12, $0
    JE euc_scalar_loop

euc_avx_loop:
    CMPQ R11, R12
    JGE euc_scalar_loop
    VMOVUPS (R8)(R11*4), Y1
    VMOVUPS (R10)(R11*4), Y2
    VSUBPS Y2, Y1, Y1
    VMULPS Y1, Y1, Y1
    VADDPS Y1, Y0, Y0
    ADDQ $8, R11
    JMP euc_avx_loop

euc_scalar_loop:
    CMPQ R11, R9
    JGE euc_done
    VMOVSS (R8)(R11*4), X1
    VMOVSS (R10)(R11*4), X2
    VSUBSS X2, X1, X1
    VMULSS X1, X1, X1
    VADDSS X1, X0, X0
    ADDQ $1, R11
    JMP euc_scalar_loop

euc_done:
    VEXTRACTF128 $1, Y0, X1
    VADDPS X1, X0, X0
    VSHUFPS $0x0E, X0, X0, X1
    VADDPS X1, X0, X0
    VSHUFPS $0x01, X0, X0, X1
    VADDSS X1, X0, X0
    VSQRTSS X0, X0, X0
    VMOVSS X0, ret+48(FP)
    VZEROUPPER
    RET


// func cosineDistanceAVX(a, b []float32) float32
TEXT ·cosineDistanceAVX(SB), NOSPLIT, $0-52
    MOVQ a_base+0(FP), R8
    MOVQ a_len+8(FP), R9
    MOVQ b_base+24(FP), R10

    VXORPS X0, X0, X0 // dot
    VXORPS X1, X1, X1 // normA
    VXORPS X2, X2, X2 // normB

    XORQ R11, R11
    MOVQ R9, R12
    ANDQ $~7, R12

    CMPQ R12, $0
    JE cos_scalar_loop

cos_avx_loop:
    CMPQ R11, R12
    JGE cos_scalar_loop
    VMOVUPS (R8)(R11*4), Y3
    VMOVUPS (R10)(R11*4), Y4

    VMULPS Y4, Y3, Y5
    VADDPS Y5, Y0, Y0

    VMULPS Y3, Y3, Y6
    VADDPS Y6, Y1, Y1

    VMULPS Y4, Y4, Y7
    VADDPS Y7, Y2, Y2

    ADDQ $8, R11
    JMP cos_avx_loop

cos_scalar_loop:
    CMPQ R11, R9
    JGE cos_done
    VMOVSS (R8)(R11*4), X3
    VMOVSS (R10)(R11*4), X4

    VMULSS X4, X3, X5
    VADDSS X5, X0, X0

    VMULSS X3, X3, X6
    VADDSS X6, X1, X1

    VMULSS X4, X4, X7
    VADDSS X7, X2, X2

    ADDQ $1, R11
    JMP cos_scalar_loop

cos_done:
    VEXTRACTF128 $1, Y0, X3
    VADDPS X3, X0, X0
    VSHUFPS $0x0E, X0, X0, X3
    VADDPS X3, X0, X0
    VSHUFPS $0x01, X0, X0, X3
    VADDSS X3, X0, X0 // X0 has dot

    VEXTRACTF128 $1, Y1, X3
    VADDPS X3, X1, X1
    VSHUFPS $0x0E, X1, X1, X3
    VADDPS X3, X1, X1
    VSHUFPS $0x01, X1, X1, X3
    VADDSS X3, X1, X1 // X1 has normA

    VEXTRACTF128 $1, Y2, X3
    VADDPS X3, X2, X2
    VSHUFPS $0x0E, X2, X2, X3
    VADDPS X3, X2, X2
    VSHUFPS $0x01, X2, X2, X3
    VADDSS X3, X2, X2 // X2 has normB

    VXORPS X3, X3, X3
    UCOMISS X1, X3
    JE zero_norm
    UCOMISS X2, X3
    JE zero_norm

    VSQRTSS X1, X1, X1
    VSQRTSS X2, X2, X2
    VMULSS X2, X1, X1
    VDIVSS X1, X0, X0

    // 1.0 - similarity
    MOVQ $0x3F800000, R11
    MOVL R11, X3
    VSUBSS X0, X3, X3

    VMOVSS X3, ret+48(FP)
    VZEROUPPER
    RET

zero_norm:
    MOVQ $0x3F800000, R11
    MOVL R11, X3
    VMOVSS X3, ret+48(FP)
    VZEROUPPER
    RET
